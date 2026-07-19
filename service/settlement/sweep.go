package settlement

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/receipt"
)

// Grace windows for the stale-order sweep. They are deliberately longer than the
// happy-path round trip so the sweep never races a task that is simply slow.
const (
	// deliveredSettleGrace: an order the provider delivered (provider receipt
	// present) but that has sat un-settled this long is settled on the provider
	// receipt alone. The normal path settles within seconds once both receipts
	// arrive; anything past this means the client side went away (page reload,
	// dropped relay) and the missing client receipt will never come.
	deliveredSettleGrace = 2 * time.Minute
	// undeliveredRefundGrace: an order stuck in a pre-delivery state with no
	// provider receipt and no active lease this long is presumed abandoned
	// (provider never ran it) and the client is refunded. Kept well above the
	// lease TTL so a live run is never refunded out from under itself.
	undeliveredRefundGrace = 10 * time.Minute
)

// deliveredStates are the states an order passes through once the provider has
// (or is about to have) delivered a result. With a provider receipt present,
// these are settleable in the provider's favor.
var deliveredStates = []string{
	model.OrderRunning, model.OrderResultReady, model.OrderVerifying,
}

// preDeliveryStates are the states before the provider delivers a result. Stuck
// here with no provider receipt, the client's reserved funds should be refunded.
var preDeliveryStates = []string{
	model.OrderFundsReserved, model.OrderMatching, model.OrderOffered,
	model.OrderReserved, model.OrderDataReady, model.OrderRunning,
}

// SweepStaleOrders is one pass of the stale-order reconciliation. It resolves
// two kinds of orders whose funds would otherwise stay frozen forever:
//
//  1. Provider delivered, client never confirmed (the reported bug): the buyer's
//     tab reloaded mid-run, so the client receipt never arrived and the order
//     is stuck in RUNNING/RESULT_READY/VERIFYING. The node consumed compute, so
//     these are settled on the provider receipt — provider and author are paid,
//     the remainder is released back to the client.
//  2. Never delivered, abandoned: stuck in a pre-delivery state with no provider
//     receipt and no active lease past a longer grace period — the client is
//     refunded in full.
//
// Returns how many orders were settled and refunded. Safe to run repeatedly;
// every underlying operation is idempotent. Master-only (mutates shared money
// state), like the lease sweeper.
func SweepStaleOrders(now int64) (settled int, refunded int, err error) {
	settled, serr := sweepDelivered(now)
	refunded, rerr := sweepUndelivered(now)
	if serr != nil {
		return settled, refunded, serr
	}
	return settled, refunded, rerr
}

// sweepDelivered settles orders the provider executed but the client never
// confirmed (missing client receipt). In this MVP task_id == order_id, attempt 1.
func sweepDelivered(now int64) (int, error) {
	cutoff := now - int64(deliveredSettleGrace.Seconds())
	orders, err := model.FindOrdersByStateOlderThan(deliveredStates, cutoff, 100)
	if err != nil {
		return 0, err
	}
	settled := 0
	for _, o := range orders {
		// Only settle when the provider actually delivered (has a receipt).
		// Without one there is no proof of execution — sweepUndelivered handles
		// the abandoned case.
		pr, gerr := model.GetReceipt(o.Id, 1, receipt.PartyProvider)
		if gerr != nil || pr == nil {
			continue
		}
		if _, serr := SettleDelivered(o.Id, o.Id, 1); serr != nil {
			common.SysError(fmt.Sprintf("stale-order settle failed for %s: %v", o.Id, serr))
			continue
		}
		settled++
	}
	return settled, nil
}

// sweepUndelivered refunds orders that never reached delivery, have no provider
// receipt, hold no active lease, and have been idle past the refund grace.
func sweepUndelivered(now int64) (int, error) {
	cutoff := now - int64(undeliveredRefundGrace.Seconds())
	orders, err := model.FindOrdersByStateOlderThan(preDeliveryStates, cutoff, 100)
	if err != nil {
		return 0, err
	}
	refunded := 0
	for _, o := range orders {
		// A provider receipt means the work was delivered — let sweepDelivered
		// settle it instead of refunding.
		if pr, _ := model.GetReceipt(o.Id, 1, receipt.PartyProvider); pr != nil {
			continue
		}
		// A live lease means a provider is (or may still be) running it; don't
		// refund out from under an in-flight task.
		if ta, _ := model.GetTaskAttempt(o.Id, 1); ta != nil {
			if lease, _ := model.GetActiveLeaseForNode(ta.NodeId); lease != nil && lease.TaskId == o.Id {
				continue
			}
		}
		if err := refundStuckOrder(o.Id); err != nil {
			common.SysError(fmt.Sprintf("stale-order refund failed for %s: %v", o.Id, err))
			continue
		}
		refunded++
	}
	return refunded, nil
}

// refundStuckOrder moves a pre-delivery order to a terminal-failure state that
// can legally reach REFUNDED, then refunds the client. FUNDS_RESERVED can only
// go to CANCELLED; the later states go through TIMED_OUT. Refund is idempotent.
func refundStuckOrder(orderId string) error {
	o, err := model.GetOrder(orderId)
	if err != nil {
		return err
	}
	switch o.State {
	case model.OrderFundsReserved:
		if _, terr := model.ApplyTransition(orderId, model.OrderCancelled, nil); terr != nil {
			return terr
		}
	case model.OrderMatching, model.OrderOffered, model.OrderReserved,
		model.OrderDataReady, model.OrderRunning:
		if _, terr := model.ApplyTransition(orderId, model.OrderTimedOut, nil); terr != nil {
			return terr
		}
	default:
		// Already terminal or not refundable from here.
		return nil
	}
	_, err = Refund(orderId)
	return err
}

// StartStaleOrderSweep runs SweepStaleOrders on an interval until stop is
// closed. Launch as a background goroutine on the master node only — it mutates
// shared order/ledger state. Complements StartLeaseExpiryLoop, which only frees
// node concurrency slots and never touches order state or funds.
func StartStaleOrderSweep(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			settled, refunded, err := SweepStaleOrders(common.GetTimestamp())
			if err != nil {
				common.SysError("stale-order sweep failed: " + err.Error())
			} else if settled > 0 || refunded > 0 {
				common.SysLog(fmt.Sprintf("stale-order sweep: settled %d, refunded %d", settled, refunded))
			}
		}
	}
}
