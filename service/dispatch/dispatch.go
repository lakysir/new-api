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
const DefaultLeaseTTL = 4 * time.Minute

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
	TargetSite      string `json:"target_site,omitempty"`
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
	targetSite := ""
	if version, versionErr := model.GetScriptVersion(o.ScriptId, o.Version); versionErr == nil && version.CategoryId > 0 {
		if category, categoryErr := model.GetScriptCategory(version.CategoryId); categoryErr == nil {
			targetSite = category.Site
		}
	}
	// Must be matching to dispatch. FUNDS_RESERVED advances to MATCHING first.
	if o.State == model.OrderFundsReserved {
		if _, err := model.ApplyTransition(orderId, model.OrderMatching, nil); err != nil {
			return nil, err
		}
	} else if o.State != model.OrderMatching {
		return nil, ErrOrderNotMatchable
	}

	candidates, err := model.ScheduleCandidates(o.ScriptId, o.Version, o.MaxAmountMicros, 10, o.ProviderGroupId)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}
	// Build the ordered list of candidates to attempt. In auto mode we try the
	// ranked candidates in turn; in chosen mode only the client's node.
	attemptOrder := candidates
	if o.ChosenNodeId != "" {
		matched := false
		for _, cnd := range candidates {
			if cnd.NodeId == o.ChosenNodeId {
				attemptOrder = []model.CandidateNode{cnd}
				matched = true
				break
			}
		}
		if !matched {
			// Chosen node not currently eligible (offline/busy): no dispatch yet.
			return nil, ErrNoCandidates
		}
	}

	taskId := orderId // 1:1 order:task in the MVP
	// Try candidates in ranked order. A node can pass the ScheduleCandidates
	// filter (state=IDLE) yet still fail reservation with ErrNodeBusy when it
	// carries a stale active lease (expired-but-not-yet-reaped, or a lost race).
	// Skip past those to the next idle candidate instead of failing the whole
	// dispatch — otherwise "auto" reports "node already has an active lease"
	// while other free nodes sit unused. A chosen node has a single-element
	// list, so it still surfaces ErrNodeBusy as before.
	var result *Result
	for _, best := range attemptOrder {
		var attemptResult *Result
		txErr := model.DB.Transaction(func(tx *gorm.DB) error {
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
				TargetSite: targetSite,
			})
			ev, eerr := model.EnqueueOutboxTx(tx, "task.offer", taskId, string(payload))
			if eerr != nil {
				return eerr
			}
			// Advance order to OFFERED with the same optimistic-lock discipline.
			if terr := transitionInTx(tx, orderId, model.OrderOffered); terr != nil {
				return terr
			}
			attemptResult = &Result{
				OrderId: orderId, TaskId: taskId, Attempt: attempt, NodeId: best.NodeId,
				LeaseId: lease.Id, EventId: ev.EventId, OfferedMicros: best.PriceMicros,
			}
			_ = ta
			return nil
		})
		if txErr == nil {
			result = attemptResult
			break
		}
		// A busy node is a soft failure: try the next ranked candidate.
		if errors.Is(txErr, model.ErrNodeBusy) {
			continue
		}
		return nil, txErr
	}
	if result == nil {
		// Every candidate turned out busy — treat as "no eligible node" so the
		// caller (CreateOrder) leaves the order matching for the async offer
		// retry rather than hard-failing the request with ErrNodeBusy.
		return nil, ErrNoCandidates
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
