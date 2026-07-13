// Package dispatch orchestrates matching a funded order to a node: it picks the
// best candidate, atomically reserves a lease + creates the task attempt +
// advances the order + enqueues the task.offer event in one transaction, so the
// reservation and its control event are always consistent (architecture §10.3,
// §22.4). No partial state can be observed.
package dispatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// DefaultLeaseTTL is the initial lease lifetime granted on reservation.
const DefaultLeaseTTL = 30 * time.Second

var (
	// ErrNoCandidates is returned when no eligible node exists for the order.
	ErrNoCandidates = errors.New("no eligible node for order")
	// ErrOrderNotMatchable is returned when the order is not in a matchable state.
	ErrOrderNotMatchable = errors.New("order is not in a matchable state")
)

// Result reports a successful dispatch.
type Result struct {
	OrderId       string
	TaskId        string
	Attempt       int
	NodeId        string
	LeaseId       string
	EventId       string
	OfferedMicros int64
}

// taskOfferPayload is the metadata-only event body (no code, no plaintext).
type taskOfferPayload struct {
	TaskId          string `json:"task_id"`
	Attempt         int    `json:"attempt"`
	OrderId         string `json:"order_id"`
	ScriptId        int    `json:"script_id"`
	ScriptVersion   int    `json:"script_version"`
	InputHash       string `json:"input_hash"`
	OfferedMicros   int64  `json:"offered_amount_micros"`
	LeaseExpiresAt  int64  `json:"lease_expires_at"`
	MaxDurationSecs int    `json:"max_duration_seconds"`
}

// Dispatch matches order to the best candidate node and reserves it. attempt is
// the execution attempt number (1 for the first try, incremented on retry). The
// whole reservation is one transaction; on node-race loss it returns
// model.ErrNodeBusy and the caller may retry with the next candidate.
func Dispatch(orderId string, attempt int) (*Result, error) {
	o, err := model.GetOrder(orderId)
	if err != nil {
		return nil, err
	}
	// Must be matching to dispatch. FUNDS_RESERVED advances to MATCHING first.
	if o.State == model.OrderFundsReserved {
		if _, err := model.ApplyTransition(orderId, model.OrderMatching, nil); err != nil {
			return nil, err
		}
	} else if o.State != model.OrderMatching {
		return nil, ErrOrderNotMatchable
	}

	candidates, err := model.ScheduleCandidates(o.ScriptId, o.Version, o.MaxAmountMicros, 10)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}
	best := candidates[0]

	taskId := orderId // 1:1 order:task in the MVP
	var result *Result
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		// Reuse the atomic reservation primitive, but within this tx we also
		// advance the order and enqueue the offer so all three commit together.
		lease, ta, rerr := reserveInTx(tx, taskId, orderId, best.NodeId, attempt, DefaultLeaseTTL)
		if rerr != nil {
			return rerr
		}
		leaseExpires := time.Now().Add(DefaultLeaseTTL).Unix()
		payload, _ := json.Marshal(taskOfferPayload{
			TaskId: taskId, Attempt: attempt, OrderId: orderId,
			ScriptId: o.ScriptId, ScriptVersion: o.Version, InputHash: o.InputHash,
			OfferedMicros: best.PriceMicros, LeaseExpiresAt: leaseExpires, MaxDurationSecs: 180,
		})
		ev, eerr := model.EnqueueOutboxTx(tx, "task.offer", taskId, string(payload))
		if eerr != nil {
			return eerr
		}
		// Advance order to OFFERED with the same optimistic-lock discipline.
		if terr := transitionInTx(tx, orderId, model.OrderOffered); terr != nil {
			return terr
		}
		result = &Result{
			OrderId: orderId, TaskId: taskId, Attempt: attempt, NodeId: best.NodeId,
			LeaseId: lease.Id, EventId: ev.EventId, OfferedMicros: best.PriceMicros,
		}
		_ = ta
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// reserveInTx creates the lease + task attempt + flips node to BUSY inside an
// existing transaction (mirrors model.ReserveNode but composable).
func reserveInTx(tx *gorm.DB, taskId, orderId, nodeId string, attempt int, ttl time.Duration) (*model.Lease, *model.TaskAttempt, error) {
	var node model.Node
	if err := tx.Where("id = ?", nodeId).First(&node).Error; err != nil {
		return nil, nil, model.ErrNodeNotFound
	}
	if node.State != model.NodeStateIdle || !node.IsOnline() {
		return nil, nil, model.ErrNodeBusy
	}
	active := true
	lease := &model.Lease{
		Id: "lea_" + model.NewEventId()[4:], NodeId: nodeId, TaskId: taskId,
		Attempt: attempt, Active: &active, ExpiresAt: time.Now().Add(ttl).Unix(),
	}
	if err := tx.Create(lease).Error; err != nil {
		if isUniqueErr(err) {
			return nil, nil, model.ErrNodeBusy
		}
		return nil, nil, err
	}
	ta := &model.TaskAttempt{TaskId: taskId, OrderId: orderId, Attempt: attempt, NodeId: nodeId, LeaseId: lease.Id, State: model.AttemptReserved}
	if err := tx.Create(ta).Error; err != nil {
		if isUniqueErr(err) {
			return nil, nil, fmt.Errorf("task attempt already exists")
		}
		return nil, nil, err
	}
	if err := tx.Model(&model.Node{}).Where("id = ?", nodeId).Update("state", model.NodeStateBusy).Error; err != nil {
		return nil, nil, err
	}
	return lease, ta, nil
}

// transitionInTx applies an order state transition with optimistic locking
// inside an existing transaction.
func transitionInTx(tx *gorm.DB, orderId, newState string) error {
	var o model.Order
	if err := tx.Where("id = ?", orderId).First(&o).Error; err != nil {
		return err
	}
	if !model.CanTransition(o.State, newState) {
		return model.ErrIllegalTransition
	}
	res := tx.Model(&model.Order{}).
		Where("id = ? AND lock_version = ?", orderId, o.LockVersion).
		Updates(map[string]any{"state": newState, "lock_version": o.LockVersion + 1, "updated_at": time.Now().Unix()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return model.ErrOrderConcurrentUpdate
	}
	return nil
}

// isUniqueErr detects a unique-constraint violation across drivers.
func isUniqueErr(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	for _, s := range []string{"UNIQUE constraint failed", "duplicate key value", "Duplicate entry", "1062"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
