package settlement

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&model.Order{}, &model.OrderPriceSnapshot{},
		&model.LedgerAccount{}, &model.LedgerTransaction{}, &model.LedgerEntry{},
		&model.ScriptVersion{}, &model.Node{}, &model.TaskAttempt{}, &model.Receipt{}, &model.UserScript{},
	); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// seedOrder creates a CREATED order with a known max amount for a client.
func seedOrder(t *testing.T, clientId int, maxMicros int64, key string) *model.Order {
	t.Helper()
	o := &model.Order{
		Id: model.NewOrderId(), ClientId: clientId, ScriptId: 1, Version: 1,
		State: model.OrderCreated, IdempotencyKey: key, MaxAmountMicros: maxMicros,
	}
	snap := &model.OrderPriceSnapshot{Currency: Currency, MaxCustomerAmountMicros: maxMicros}
	got, _, err := model.CreateOrderWithSnapshot(o, snap)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// walkToVerifying advances a FUNDS_RESERVED order through the happy path to
// VERIFYING, the natural pre-settlement state (mirrors the execution lifecycle).
func walkToVerifying(t *testing.T, orderId string) {
	t.Helper()
	for _, st := range []string{
		model.OrderMatching, model.OrderOffered, model.OrderReserved,
		model.OrderDataReady, model.OrderRunning, model.OrderResultReady,
		model.OrderVerifying,
	} {
		if _, err := model.ApplyTransition(orderId, st, nil); err != nil {
			t.Fatalf("advance to %s: %v", st, err)
		}
	}
}

func TestFullSettlementClosedLoop(t *testing.T) {
	clientId := 1001
	if _, err := Deposit(clientId, 200000, "dep-1001"); err != nil {
		t.Fatal(err)
	}
	o := seedOrder(t, clientId, 112000, "order-loop-1")

	if _, err := ReserveFunds(o.Id); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	// (balances checked below before walking the lifecycle)
	// After reserve: available 88000, reserved 112000.
	avail, _ := model.GetBalance(model.OwnerClient, clientId, model.KindAvailable, Currency)
	reserved, _ := model.GetBalance(model.OwnerClient, clientId, model.KindReserved, Currency)
	if avail != 88000 || reserved != 112000 {
		t.Fatalf("post-reserve balances wrong: avail=%d reserved=%d", avail, reserved)
	}

	walkToVerifying(t, o.Id)
	// Settle: provider 100000, author 3000, platform 8000, network 1000 = 112000.
	_, err := Settle(o.Id, SettleParticipants{
		ProviderId: 2001, AuthorId: 3001,
		ProviderMicros: 100000, AuthorMicros: 3000, PlatformMicros: 8000, NetworkReserveMicros: 1000,
	})
	if err != nil {
		t.Fatalf("settle: %v", err)
	}

	reserved, _ = model.GetBalance(model.OwnerClient, clientId, model.KindReserved, Currency)
	providerBal, _ := model.GetBalance(model.OwnerProvider, 2001, model.KindPayable, Currency)
	authorBal, _ := model.GetBalance(model.OwnerAuthor, 3001, model.KindPayable, Currency)
	if reserved != 0 {
		t.Fatalf("reserved must be fully consumed, got %d", reserved)
	}
	// Provider/author use per-test unique owner ids so they are isolated; the
	// platform revenue account is a shared singleton, so it is validated by the
	// global balance invariant rather than an absolute equality here.
	if providerBal != 100000 || authorBal != 3000 {
		t.Fatalf("payout wrong: provider=%d author=%d", providerBal, authorBal)
	}

	final, _ := model.GetOrder(o.Id)
	if final.State != model.OrderSettled {
		t.Fatalf("order should be SETTLED, got %s", final.State)
	}
	if final.FinalAmountMicros != 112000 {
		t.Fatalf("final amount should be 112000, got %d", final.FinalAmountMicros)
	}
	if err := model.AssertLedgerBalanced(); err != nil {
		t.Fatalf("ledger unbalanced after settlement: %v", err)
	}
}

func TestSettlementReleasesRemainder(t *testing.T) {
	clientId := 1002
	_, _ = Deposit(clientId, 200000, "dep-1002")
	o := seedOrder(t, clientId, 112000, "order-loop-2")
	_, _ = ReserveFunds(o.Id)
	walkToVerifying(t, o.Id)

	// Actual payout less than reserved: only provider 50000 + platform 5000.
	if _, err := Settle(o.Id, SettleParticipants{
		ProviderId: 2002, ProviderMicros: 50000, PlatformMicros: 5000,
	}); err != nil {
		t.Fatal(err)
	}
	// Remainder 112000 - 55000 = 57000 returns to available: 88000 + 57000.
	avail, _ := model.GetBalance(model.OwnerClient, clientId, model.KindAvailable, Currency)
	if avail != 145000 {
		t.Fatalf("expected available 145000 after remainder release, got %d", avail)
	}
	if err := model.AssertLedgerBalanced(); err != nil {
		t.Fatal(err)
	}
}

func TestRefundReturnsFunds(t *testing.T) {
	clientId := 1003
	_, _ = Deposit(clientId, 200000, "dep-1003")
	o := seedOrder(t, clientId, 112000, "order-loop-3")
	_, _ = ReserveFunds(o.Id)
	// Drive to a refundable state: FUNDS_RESERVED -> MATCHING -> CANCELLED.
	if _, err := model.ApplyTransition(o.Id, model.OrderMatching, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := model.ApplyTransition(o.Id, model.OrderCancelled, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Refund(o.Id); err != nil {
		t.Fatalf("refund: %v", err)
	}
	avail, _ := model.GetBalance(model.OwnerClient, clientId, model.KindAvailable, Currency)
	reserved, _ := model.GetBalance(model.OwnerClient, clientId, model.KindReserved, Currency)
	if avail != 200000 || reserved != 0 {
		t.Fatalf("refund should restore full balance: avail=%d reserved=%d", avail, reserved)
	}
	final, _ := model.GetOrder(o.Id)
	if final.State != model.OrderRefunded {
		t.Fatalf("order should be REFUNDED, got %s", final.State)
	}
}

func TestReserveFailsWithoutFunds(t *testing.T) {
	clientId := 1004 // no deposit
	o := seedOrder(t, clientId, 50000, "order-loop-4")
	if _, err := ReserveFunds(o.Id); err != model.ErrInsufficientBalance {
		t.Fatalf("reserve without funds must fail, got %v", err)
	}
	// Order must remain in CREATED, not advanced.
	got, _ := model.GetOrder(o.Id)
	if got.State != model.OrderCreated {
		t.Fatalf("order state must stay CREATED on failed reserve, got %s", got.State)
	}
}

func TestSettleIdempotent(t *testing.T) {
	clientId := 1005
	_, _ = Deposit(clientId, 200000, "dep-1005")
	o := seedOrder(t, clientId, 100000, "order-loop-5")
	_, _ = ReserveFunds(o.Id)
	walkToVerifying(t, o.Id)
	p := SettleParticipants{ProviderId: 2005, ProviderMicros: 100000}
	if _, err := Settle(o.Id, p); err != nil {
		t.Fatal(err)
	}
	// Re-settling posts no new entries (idempotent ledger key); balance stable.
	_, _ = Settle(o.Id, p)
	providerBal, _ := model.GetBalance(model.OwnerProvider, 2005, model.KindPayable, Currency)
	if providerBal != 100000 {
		t.Fatalf("double settle must pay once, got %d", providerBal)
	}
}
