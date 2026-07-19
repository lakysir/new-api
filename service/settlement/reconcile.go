package settlement

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/receipt"
)

// ErrReceiptsIncomplete is returned when both parties' receipts are not yet in.
var ErrReceiptsIncomplete = errors.New("both party receipts are required")

// ReconcileResult reports what ReconcileAndSettle did.
type ReconcileResult struct {
	Matched bool
	Order   *model.Order
	Reason  string
}

// ReconcileAndSettle loads both parties' stored receipts for a task attempt,
// compares them, and either settles the order (on match) or routes it to
// dispute (on mismatch). Payout amounts come from the frozen order price
// snapshot; provider is the node owner, author is the script version author.
// Idempotent via the underlying ledger and state-machine guards.
func ReconcileAndSettle(orderId, taskId string, attempt int) (*ReconcileResult, error) {
	o, err := model.GetOrder(orderId)
	if err != nil {
		return nil, err
	}
	providerReceipt, err := model.GetReceipt(taskId, attempt, receipt.PartyProvider)
	if err != nil {
		return nil, err
	}
	clientReceipt, err := model.GetReceipt(taskId, attempt, receipt.PartyClient)
	if err != nil {
		return nil, err
	}
	if providerReceipt == nil || clientReceipt == nil {
		return nil, ErrReceiptsIncomplete
	}

	rec := receipt.Reconcile(
		receipt.Receipt{TaskId: taskId, Attempt: attempt, ResultHash: providerReceipt.ResultHash},
		receipt.Receipt{TaskId: taskId, Attempt: attempt, ResultHash: clientReceipt.ResultHash},
	)
	// Resolve the executing node and any script-reported balance up front.
	var execNodeId string
	var scriptBalance *int
	if ta, _ := model.GetTaskAttempt(taskId, attempt); ta != nil {
		execNodeId = ta.NodeId
		scriptBalance = ta.ScriptBalance
	}

	if !rec.Match {
		// Mismatch → dispute (business outcome, not an error) and count a failure
		// against the executing node, lowering its scheduling success rate.
		if execNodeId != "" {
			_ = model.RecordTaskOutcome(execNodeId, false)
			_ = model.FinalizeTaskAttempt(taskId, attempt, model.AttemptFailed)
		}
		updated, terr := model.ApplyTransition(orderId, model.OrderDisputed, nil)
		if terr != nil && terr != model.ErrIllegalTransition {
			return nil, terr
		}
		if updated == nil {
			updated = o
		}
		return &ReconcileResult{Matched: false, Order: updated, Reason: rec.Reason}, nil
	}

	// Resolve payout participants and amounts from persisted facts.
	participants, err := resolveParticipants(o, taskId, attempt)
	if err != nil {
		return nil, err
	}
	alreadySettled := o.State == model.OrderSettled
	settled, err := Settle(orderId, participants)
	if err != nil {
		return nil, err
	}
	if execNodeId != "" && !alreadySettled {
		recordSettledOutcome(o, taskId, attempt, execNodeId, scriptBalance)
	}
	return &ReconcileResult{Matched: true, Order: settled}, nil
}

// SettleDelivered settles an order that the provider demonstrably executed
// (a stored provider receipt) but for which the client receipt never arrived —
// typically because the buyer's browser tab reloaded or lost its relay
// connection mid-run. The node consumed compute, so the provider and author are
// paid from the frozen snapshot and any remainder is released back to the
// client; leaving the funds frozen forever would be the wrong outcome.
//
// It bottoms out the state machine to VERIFYING if needed (RESULT_READY carries
// the same "provider delivered" meaning), settles, and records the success-side
// node stats exactly as a normal reconciliation would. Idempotent via Settle's
// ledger key. Returns ErrReceiptsIncomplete if the provider receipt is absent —
// without proof of execution there is nothing to settle on.
func SettleDelivered(orderId, taskId string, attempt int) (*ReconcileResult, error) {
	o, err := model.GetOrder(orderId)
	if err != nil {
		return nil, err
	}
	if o.State == model.OrderSettled {
		return &ReconcileResult{Matched: true, Order: o}, nil
	}
	providerReceipt, err := model.GetReceipt(taskId, attempt, receipt.PartyProvider)
	if err != nil {
		return nil, err
	}
	if providerReceipt == nil {
		return nil, ErrReceiptsIncomplete
	}
	// Advance a still-running order into the settleable window. The provider
	// receipt is the proof of delivery, so the missing client receipt does not
	// block the transition.
	if o.State == model.OrderRunning {
		_, _ = model.ApplyTransition(orderId, model.OrderResultReady, nil)
	}
	if refreshed, gerr := model.GetOrder(orderId); gerr == nil {
		o = refreshed
	}
	if o.State == model.OrderResultReady {
		_, _ = model.ApplyTransition(orderId, model.OrderVerifying, nil)
	}

	var execNodeId string
	var scriptBalance *int
	if ta, _ := model.GetTaskAttempt(taskId, attempt); ta != nil {
		execNodeId = ta.NodeId
		scriptBalance = ta.ScriptBalance
	}
	participants, err := resolveParticipants(o, taskId, attempt)
	if err != nil {
		return nil, err
	}
	settled, err := Settle(orderId, participants)
	if err != nil {
		return nil, err
	}
	if execNodeId != "" {
		recordSettledOutcome(o, taskId, attempt, execNodeId, scriptBalance)
	}
	return &ReconcileResult{Matched: true, Order: settled}, nil
}

// recordSettledOutcome writes the success-side bookkeeping after an order is
// paid out: raise the node's scheduling success rate, mark the attempt
// succeeded, roll the capability's reported balance forward, and bump the daily
// counter. Each underlying call is idempotent.
func recordSettledOutcome(o *model.Order, taskId string, attempt int, execNodeId string, scriptBalance *int) {
	_ = model.RecordTaskOutcome(execNodeId, true)
	_ = model.FinalizeTaskAttempt(taskId, attempt, model.AttemptSucceeded)
	if scriptBalance != nil {
		_ = model.UpdateCapabilityBalance(execNodeId, o.ScriptId, o.Version, *scriptBalance)
	}
	_ = model.IncrementDailyUsed(execNodeId, o.ScriptId, o.Version)
}

// resolveParticipants derives the settle split from the order snapshot, the
// executing node's owner (provider) and the script version author.
func resolveParticipants(o *model.Order, taskId string, attempt int) (SettleParticipants, error) {
	snap, err := model.GetOrderPriceSnapshot(o.Id)
	if err != nil {
		return SettleParticipants{}, fmt.Errorf("price snapshot missing: %w", err)
	}
	ta, err := model.GetTaskAttempt(taskId, attempt)
	if err != nil {
		return SettleParticipants{}, err
	}
	if ta == nil {
		return SettleParticipants{}, errors.New("task attempt not found")
	}
	node, err := model.GetNode(ta.NodeId)
	if err != nil {
		return SettleParticipants{}, err
	}
	// Author from the fixed script version (use GetScriptVersion so a later
	// revocation does not block settling an already-executed order).
	sv, err := model.GetScriptVersion(o.ScriptId, o.Version)
	if err != nil {
		return SettleParticipants{}, err
	}
	return SettleParticipants{
		ProviderId:           node.UserId,
		AuthorId:             sv.AuthorId,
		ProviderMicros:       snap.ProviderAmountMicros,
		AuthorMicros:         snap.AuthorAmountMicros,
		PlatformMicros:       snap.PlatformFeeMicros,
		NetworkReserveMicros: snap.RelayFeeReservedMicros + snap.StorageFeeReservedMicros + snap.RiskReserveMicros,
	}, nil
}
