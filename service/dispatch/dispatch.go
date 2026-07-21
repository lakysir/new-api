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

// ErrNodeCoolingDown is returned when the only otherwise-eligible node(s) for an
// order are within the script's min-interval cooldown for its target-site
// category. It carries the seconds until the soonest node is dispatchable again
// so the caller can tell the buyer to retry, distinct from an all-busy/offline
// ErrNoCandidates. It unwraps to ErrNoCandidates so existing callers that only
// branch on "nothing matched" keep working unchanged.
type ErrNodeCoolingDown struct {
	RetryAfterSecs int64
}

func (e *ErrNodeCoolingDown) Error() string {
	return fmt.Sprintf("all eligible nodes are cooling down; retry in %ds", e.RetryAfterSecs)
}

// Unwrap lets errors.Is(err, ErrNoCandidates) still match a cooldown result.
func (e *ErrNodeCoolingDown) Unwrap() error { return ErrNoCandidates }

// noCandidateErr classifies an empty candidate set. When every otherwise-usable
// node for the order is merely within its min-interval cooldown, it returns an
// ErrNodeCoolingDown carrying the seconds until the soonest one is dispatchable
// again (min 1) so the caller can offer a retry. Otherwise (all busy/offline,
// or no matching capability at all) it returns the plain ErrNoCandidates.
func noCandidateErr(scriptId, version int, providerGroupId, chosenNodeId string, clientUserId int) error {
	readyAt, cooling := model.EarliestCooldownReadyForScript(scriptId, version, providerGroupId, chosenNodeId, clientUserId)
	if !cooling {
		return ErrNoCandidates
	}
	retryAfter := readyAt - time.Now().Unix()
	if retryAfter < 1 {
		retryAfter = 1
	}
	return &ErrNodeCoolingDown{RetryAfterSecs: retryAfter}
}

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
	// ConsumeMultiplier is the buyer's units-of-work coefficient (min 1). It is
	// authoritative (set from the paid order, not the E2EE config) so the plugin
	// merges it into the config the script reads; the script decides what it
	// means (e.g. seconds of video, number of images).
	ConsumeMultiplier int64 `json:"consume_multiplier"`
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
	categoryId := 0
	minInterval := 0
	if version, versionErr := model.GetScriptVersion(o.ScriptId, o.Version); versionErr == nil {
		categoryId = version.CategoryId
		minInterval = version.MinIntervalSeconds
		if version.CategoryId > 0 {
			if category, categoryErr := model.GetScriptCategory(version.CategoryId); categoryErr == nil {
				targetSite = category.Site
			}
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

	candidates, err := model.ScheduleCandidates(o.ScriptId, o.Version, o.MaxAmountMicros, 10, o.ProviderGroupId, o.ConsumeMultiplier, o.ClientId, o.ChosenNodeId)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, noCandidateErr(o.ScriptId, o.Version, o.ProviderGroupId, o.ChosenNodeId, o.ClientId)
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
			// Chosen node not currently eligible (offline/busy/cooling): no dispatch yet.
			return nil, noCandidateErr(o.ScriptId, o.Version, o.ProviderGroupId, o.ChosenNodeId, o.ClientId)
		}
	}

	taskId := orderId // 1:1 order:task in the MVP
	// Try candidates in ranked order. A node can pass ScheduleCandidates yet
	// still fail reservation when a concurrent request grabs the last slot; skip
	// past those to the next candidate. A chosen node has a single-element list
	// so it still surfaces ErrNodeBusy as before.
	var result *Result
	for _, best := range attemptOrder {
		var attemptResult *Result
		txErr := model.DB.Transaction(func(tx *gorm.DB) error {
			lease, ta, rerr := reserveInTx(tx, taskId, orderId, best.NodeId, o.ScriptId, o.Version, categoryId, minInterval, attempt, DefaultLeaseTTL)
			if rerr != nil {
				return rerr
			}
			leaseExpires := time.Now().Add(DefaultLeaseTTL).Unix()
			multiplier := o.ConsumeMultiplier
			if multiplier < 1 {
				multiplier = 1
			}
			payload, _ := json.Marshal(taskOfferPayload{
				TaskId: taskId, Attempt: attempt, OrderId: orderId,
				ScriptId: o.ScriptId, ScriptVersion: o.Version, InputHash: o.InputHash,
				OfferedMicros: best.PriceMicros, LeaseExpiresAt: leaseExpires, MaxDurationSecs: 180,
				TargetSite: targetSite, ConsumeMultiplier: multiplier,
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
		// Every candidate lost the reservation race — busy, or newly inside the
		// min-interval cooldown by the time we reserved. Classify so a cooldown
		// surfaces as ErrNodeCoolingDown (retryable) rather than a generic
		// ErrNoCandidates; both leave the order in MATCHING for the caller.
		return nil, noCandidateErr(o.ScriptId, o.Version, o.ProviderGroupId, o.ChosenNodeId, o.ClientId)
	}
	return result, nil
}

// reserveInTx creates the lease + task attempt + flips node to BUSY inside an
// existing transaction. With concurrent execution the node may accept multiple
// tasks up to its total concurrency; capacity is checked transactionally here.
func reserveInTx(tx *gorm.DB, taskId, orderId, nodeId string, scriptId, version, categoryId, minInterval, attempt int, ttl time.Duration) (*model.Lease, *model.TaskAttempt, error) {
	var node model.Node
	if err := tx.Where("id = ?", nodeId).First(&node).Error; err != nil {
		return nil, nil, model.ErrNodeNotFound
	}
	if !node.IsOnline() {
		return nil, nil, model.ErrNodeBusy
	}
	// Min-interval cooldown re-check inside the transaction: a candidate can pass
	// the scheduler's pre-filter yet still be within the gap by the time we
	// reserve (or a concurrent dispatch may have just started a task for this
	// category). Treat cooling as a soft busy so the caller skips to the next
	// candidate, mirroring the concurrency guards below.
	if cooling, _ := model.NodeCategoryCoolingUntil(tx, nodeId, categoryId, minInterval); cooling {
		return nil, nil, model.ErrNodeBusy
	}
	// Node-level capacity: active leases < sum of all capability concurrency.
	var totalCap int64
	tx.Table("node_capabilities").
		Select("COALESCE(SUM(concurrency), 1)").
		Where("node_id = ? AND status = ?", nodeId, model.CapabilityStatusActive).
		Scan(&totalCap)
	if totalCap < 1 {
		totalCap = 1
	}
	var totalActive int64
	tx.Model(&model.Lease{}).Where("node_id = ? AND active = ?", nodeId, true).Count(&totalActive)
	if totalActive >= totalCap {
		return nil, nil, model.ErrNodeBusy
	}
	// Per-script capacity: active leases for this script < this cap's concurrency.
	var scriptCap int64
	tx.Table("node_capabilities").
		Select("COALESCE(concurrency, 1)").
		Where("node_id = ? AND script_id = ? AND version = ? AND status = ?", nodeId, scriptId, version, model.CapabilityStatusActive).
		Scan(&scriptCap)
	if scriptCap < 1 {
		scriptCap = 1
	}
	var scriptActive int64
	tx.Model(&model.Lease{}).
		Where("node_id = ? AND script_id = ? AND version = ? AND active = ?", nodeId, scriptId, version, true).
		Count(&scriptActive)
	if scriptActive >= scriptCap {
		return nil, nil, model.ErrNodeBusy
	}

	active := true
	lease := &model.Lease{
		Id: "lea_" + model.NewEventId()[4:], NodeId: nodeId, TaskId: taskId,
		ScriptId: scriptId, Version: version, CategoryId: categoryId,
		Attempt: attempt, Active: &active, ExpiresAt: time.Now().Add(ttl).Unix(),
	}
	if err := tx.Create(lease).Error; err != nil {
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
