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
	// Resolve the executing node up front so we can record its outcome.
	var execNodeId string
	if ta, _ := model.GetTaskAttempt(taskId, attempt); ta != nil {
		execNodeId = ta.NodeId
	}

	if !rec.Match {
		// Mismatch → dispute (business outcome, not an error) and count a failure
		// against the executing node, lowering its scheduling success rate.
		if execNodeId != "" {
			_ = model.RecordTaskOutcome(execNodeId, false)
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
	settled, err := Settle(orderId, participants)
	if err == nil && execNodeId != "" && o.State != model.OrderSettled {
		// Success raises the node's scheduling success rate.
		_ = model.RecordTaskOutcome(execNodeId, true)
		_ = model.ConsumeCapabilityQuota(execNodeId, o.ScriptId, o.Version)
	}
	if err != nil {
		return nil, err
	}
	return &ReconcileResult{Matched: true, Order: settled}, nil
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
