package settlement

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/receipt"
)

// touchOrderUpdatedAt back-dates an order's updated_at so the sweep sees it as
// stale (the sweep keys off idle time since the last state change).
func touchOrderUpdatedAt(t *testing.T, orderId string, updatedAt int64) {
	t.Helper()
	if err := model.DB.Model(&model.Order{}).Where("id = ?", orderId).
		Update("updated_at", updatedAt).Error; err != nil {
		t.Fatal(err)
	}
}

// TestSweepSettlesDeliveredStuckOrder is the end-to-end regression for the
// reported bug: a provider-delivered order stuck in VERIFYING (client receipt
// never arrived because the buyer reloaded) is settled by the sweep, not left
// frozen.
func TestSweepSettlesDeliveredStuckOrder(t *testing.T) {
	orderId, taskId := setupExecutedOrder(t, 6201, 7201, 8201, "sweep-delivered")
	saveReceipt(t, orderId, taskId, receipt.PartyProvider, "sha256:done")
	// Stuck long enough to cross the delivered grace window.
	touchOrderUpdatedAt(t, orderId, 1)

	settled, refunded, err := SweepStaleOrders(1 + int64(deliveredSettleGrace.Seconds()) + 1)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 1 || refunded != 0 {
		t.Fatalf("expected settled=1 refunded=0, got settled=%d refunded=%d", settled, refunded)
	}
	o, _ := model.GetOrder(orderId)
	if o.State != model.OrderSettled {
		t.Fatalf("order must be settled, got %s", o.State)
	}
	prov, _ := model.GetBalance(model.OwnerProvider, 7201, model.KindPayable, Currency)
	if prov != 100000 {
		t.Fatalf("provider must be paid, got %d", prov)
	}
	if err := model.AssertLedgerBalanced(); err != nil {
		t.Fatal(err)
	}
}

// TestSweepRefundsAbandonedOrder covers the mirror case: a pre-delivery order
// with no provider receipt and no active lease, idle past the refund grace, is
// refunded so the buyer's funds are not frozen.
func TestSweepRefundsAbandonedOrder(t *testing.T) {
	o := seedOrder(t, 6202, 90000, "sweep-abandoned")
	_, _ = Deposit(6202, 200000, "dep-sweep-abandoned")
	if _, err := ReserveFunds(o.Id); err != nil {
		t.Fatal(err)
	}
	// Advance to MATCHING (a pre-delivery state) then back-date it.
	if _, err := model.ApplyTransition(o.Id, model.OrderMatching, nil); err != nil {
		t.Fatal(err)
	}
	touchOrderUpdatedAt(t, o.Id, 1)

	settled, refunded, err := SweepStaleOrders(1 + int64(undeliveredRefundGrace.Seconds()) + 1)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 0 || refunded != 1 {
		t.Fatalf("expected settled=0 refunded=1, got settled=%d refunded=%d", settled, refunded)
	}
	got, _ := model.GetOrder(o.Id)
	if got.State != model.OrderRefunded {
		t.Fatalf("order must be refunded, got %s", got.State)
	}
	avail, _ := model.GetBalance(model.OwnerClient, 6202, model.KindAvailable, Currency)
	if avail != 200000 {
		t.Fatalf("buyer must be made whole, got %d", avail)
	}
	if err := model.AssertLedgerBalanced(); err != nil {
		t.Fatal(err)
	}
}

// TestSweepSkipsFreshOrders ensures the sweep never touches orders still within
// their grace window (no premature settle/refund of a live task).
func TestSweepSkipsFreshOrders(t *testing.T) {
	orderId, taskId := setupExecutedOrder(t, 6203, 7203, 8203, "sweep-fresh")
	saveReceipt(t, orderId, taskId, receipt.PartyProvider, "sha256:done")
	touchOrderUpdatedAt(t, orderId, 1_000_000)

	// "now" only just after the update: inside every grace window.
	settled, refunded, err := SweepStaleOrders(1_000_001)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 0 || refunded != 0 {
		t.Fatalf("fresh order must be untouched, got settled=%d refunded=%d", settled, refunded)
	}
}
