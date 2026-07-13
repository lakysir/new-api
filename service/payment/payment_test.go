package payment

import (
	"context"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const cur = "USD_TEST"

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
		&model.LedgerAccount{}, &model.LedgerTransaction{}, &model.LedgerEntry{},
		&model.PaymentTransaction{},
	); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestFakeAdapterDepositCursorIdempotent(t *testing.T) {
	a := NewFakeAdapter("fake", 100)
	a.InjectDeposit(DepositEvent{ExternalId: "d1", AccountId: "10", AmountMicros: 5000, Confirmations: 3})
	batch, cursor, err := a.ScanDeposits(context.Background(), "")
	if err != nil || len(batch) != 1 {
		t.Fatalf("expected 1 event, got %d err=%v", len(batch), err)
	}
	// Re-scan from the returned cursor: no duplicates.
	batch2, _, _ := a.ScanDeposits(context.Background(), cursor)
	if len(batch2) != 0 {
		t.Fatalf("re-scan must be empty, got %d", len(batch2))
	}
}

func TestProcessDepositsCreditsLedgerOnce(t *testing.T) {
	a := NewFakeAdapter("fake", 100)
	a.InjectDeposit(DepositEvent{ExternalId: "dep-A", AccountId: "20", AmountMicros: 70000, Confirmations: 3})
	svc := NewService(a, cur, 1)

	credited, cursor, err := svc.ProcessDeposits(context.Background(), "")
	if err != nil || credited != 1 {
		t.Fatalf("expected 1 credited, got %d err=%v", credited, err)
	}
	bal, _ := model.GetBalance(model.OwnerClient, 20, model.KindAvailable, cur)
	if bal != 70000 {
		t.Fatalf("expected balance 70000, got %d", bal)
	}
	// Re-process same deposit (duplicate callback simulation): no double credit.
	a2 := NewFakeAdapter("fake", 100)
	a2.InjectDeposit(DepositEvent{ExternalId: "dep-A", AccountId: "20", AmountMicros: 70000, Confirmations: 3})
	svc2 := NewService(a2, cur, 1)
	if _, _, err := svc2.ProcessDeposits(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	bal, _ = model.GetBalance(model.OwnerClient, 20, model.KindAvailable, cur)
	if bal != 70000 {
		t.Fatalf("duplicate deposit must not double credit, got %d", bal)
	}
	_ = cursor
}

func TestUnconfirmedDepositNotCredited(t *testing.T) {
	a := NewFakeAdapter("fake", 100)
	a.InjectDeposit(DepositEvent{ExternalId: "dep-pending", AccountId: "21", AmountMicros: 1000, Confirmations: 0})
	svc := NewService(a, cur, 2)
	credited, _, _ := svc.ProcessDeposits(context.Background(), "")
	if credited != 0 {
		t.Fatalf("unconfirmed deposit must not be credited, got %d", credited)
	}
}

func TestWithdrawalBelowDynamicMinimumRejected(t *testing.T) {
	// Fee 100; ratio 10 => min amount 1000. Give provider 500 payable.
	_, _ = model.PostTransaction("seed-pw-1", model.TxSettle, "r", []model.EntryInput{
		{OwnerType: model.OwnerExternal, OwnerId: 0, Kind: model.KindSource, Currency: cur, Direction: model.DirDebit, Amount: 500},
		{OwnerType: model.OwnerProvider, OwnerId: 30, Kind: model.KindPayable, Currency: cur, Direction: model.DirCredit, Amount: 500},
	})
	svc := NewService(NewFakeAdapter("fake", 100), cur, 1)
	_, _, err := svc.RequestWithdrawal(context.Background(), WithdrawInput{
		OwnerType: model.OwnerProvider, OwnerId: 30, ToAddress: "addr", AmountMicros: 500,
	})
	if err != ErrBelowMinimum {
		t.Fatalf("expected ErrBelowMinimum, got %v", err)
	}
}

func TestWithdrawalDebitsPayableAndSubmits(t *testing.T) {
	// Provider 31 has 200000 payable.
	_, _ = model.PostTransaction("seed-pw-2", model.TxSettle, "r", []model.EntryInput{
		{OwnerType: model.OwnerExternal, OwnerId: 0, Kind: model.KindSource, Currency: cur, Direction: model.DirDebit, Amount: 200000},
		{OwnerType: model.OwnerProvider, OwnerId: 31, Kind: model.KindPayable, Currency: cur, Direction: model.DirCredit, Amount: 200000},
	})
	svc := NewService(NewFakeAdapter("fake", 100), cur, 1)
	wd, fee, err := svc.RequestWithdrawal(context.Background(), WithdrawInput{
		OwnerType: model.OwnerProvider, OwnerId: 31, ToAddress: "addr", AmountMicros: 120000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if wd.ExternalId == "" || fee.FeeMicros != 100 {
		t.Fatalf("unexpected withdrawal/fee: %+v %+v", wd, fee)
	}
	// Payable reduced by the withdrawn amount.
	bal, _ := model.GetBalance(model.OwnerProvider, 31, model.KindPayable, cur)
	if bal != 80000 {
		t.Fatalf("expected payable 80000 after withdrawal, got %d", bal)
	}
	if err := model.AssertLedgerBalanced(); err != nil {
		t.Fatal(err)
	}
}

func TestWithdrawalInsufficientBalance(t *testing.T) {
	svc := NewService(NewFakeAdapter("fake", 100), cur, 1)
	_, _, err := svc.RequestWithdrawal(context.Background(), WithdrawInput{
		OwnerType: model.OwnerProvider, OwnerId: 999, ToAddress: "addr", AmountMicros: 5000,
	})
	if err != ErrInsufficientWithdrawable {
		t.Fatalf("expected ErrInsufficientWithdrawable, got %v", err)
	}
}
