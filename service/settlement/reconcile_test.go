package settlement

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/receipt"
)

// setupExecutedOrder builds an order walked to VERIFYING with funds reserved, a
// task attempt bound to a provider node, a script version (author) and a price
// snapshot — the state right before receipts arrive.
func setupExecutedOrder(t *testing.T, clientId, providerUserId, authorId int, key string) (orderId, taskId string) {
	t.Helper()
	// Script version with a known author.
	sv := &model.ScriptVersion{ScriptId: 5000 + clientId, AuthorId: authorId, CodeSha256: "sha256:x", ReviewStatus: model.ScriptVersionApproved}
	if err := model.CreateScriptVersion(sv); err != nil {
		t.Fatal(err)
	}
	// Provider node owned by providerUserId.
	nodeId := "node_settle_" + key
	if err := model.DB.Create(&model.Node{Id: nodeId, DeviceId: "d", UserId: providerUserId, State: model.NodeStateBusy, LastSeenAt: 1}).Error; err != nil {
		t.Fatal(err)
	}

	_, _ = Deposit(clientId, 200000, "dep-"+key)
	o := &model.Order{
		Id: model.NewOrderId(), ClientId: clientId, ScriptId: sv.ScriptId, Version: sv.Version,
		State: model.OrderCreated, IdempotencyKey: key, MaxAmountMicros: 112000,
	}
	snap := &model.OrderPriceSnapshot{
		Currency: Currency, ProviderAmountMicros: 100000, AuthorAmountMicros: 3000,
		PlatformFeeMicros: 8000, RiskReserveMicros: 1000, MaxCustomerAmountMicros: 112000,
	}
	created, _, err := model.CreateOrderWithSnapshot(o, snap)
	if err != nil {
		t.Fatal(err)
	}
	taskId = created.Id // 1:1 order:task in the MVP
	if _, err := ReserveFunds(created.Id); err != nil {
		t.Fatal(err)
	}
	walkToVerifying(t, created.Id)

	// Task attempt bound to the provider node.
	if err := model.DB.Create(&model.TaskAttempt{
		TaskId: taskId, OrderId: created.Id, Attempt: 1, NodeId: nodeId, State: model.AttemptResultReady,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return created.Id, taskId
}

func saveReceipt(t *testing.T, orderId, taskId, party, resultHash string) {
	t.Helper()
	if err := model.SaveReceipt(&model.Receipt{
		TaskId: taskId, Attempt: 1, Party: party, OrderId: orderId, ResultHash: resultHash,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileMatchSettles(t *testing.T) {
	orderId, taskId := setupExecutedOrder(t, 6001, 7001, 8001, "recon-match")
	saveReceipt(t, orderId, taskId, receipt.PartyProvider, "sha256:same")

	// Only one receipt yet: incomplete.
	if _, err := ReconcileAndSettle(orderId, taskId, 1); err != ErrReceiptsIncomplete {
		t.Fatalf("expected incomplete, got %v", err)
	}
	saveReceipt(t, orderId, taskId, receipt.PartyClient, "sha256:same")

	res, err := ReconcileAndSettle(orderId, taskId, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Order.State != model.OrderSettled {
		t.Fatalf("matching receipts must settle, got matched=%v state=%s", res.Matched, res.Order.State)
	}
	// Provider (node owner) and author paid from the snapshot.
	prov, _ := model.GetBalance(model.OwnerProvider, 7001, model.KindPayable, Currency)
	auth, _ := model.GetBalance(model.OwnerAuthor, 8001, model.KindPayable, Currency)
	if prov != 100000 || auth != 3000 {
		t.Fatalf("payout wrong: provider=%d author=%d", prov, auth)
	}
	if err := model.AssertLedgerBalanced(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileMismatchDisputes(t *testing.T) {
	orderId, taskId := setupExecutedOrder(t, 6002, 7002, 8002, "recon-mismatch")
	saveReceipt(t, orderId, taskId, receipt.PartyProvider, "sha256:aaa")
	saveReceipt(t, orderId, taskId, receipt.PartyClient, "sha256:bbb")

	res, err := ReconcileAndSettle(orderId, taskId, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched {
		t.Fatal("mismatched receipts must not settle")
	}
	if res.Order.State != model.OrderDisputed {
		t.Fatalf("mismatch must route to DISPUTED, got %s", res.Order.State)
	}
	// No payout on dispute.
	prov, _ := model.GetBalance(model.OwnerProvider, 7002, model.KindPayable, Currency)
	if prov != 0 {
		t.Fatalf("no payout expected on dispute, got %d", prov)
	}
}
