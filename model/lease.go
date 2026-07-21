// Task attempts and leases. The lease is the fact source for "a node runs at
// most one task at a time" (PRD N-006). A partial unique index on
// (node_id) WHERE active enforces it at the database; the reservation happens
// in a single transaction that also flips the node to BUSY (architecture §10.3).
package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Task attempt states track a single execution instance.
const (
	AttemptReserved    = "RESERVED"
	AttemptDataReady   = "DATA_READY"
	AttemptRunning     = "RUNNING"
	AttemptResultReady = "RESULT_READY"
	AttemptSucceeded   = "SUCCEEDED"
	AttemptFailed      = "FAILED"
	AttemptExpired     = "EXPIRED"
)

var (
	// ErrNodeBusy is returned when a node already holds an active lease.
	ErrNodeBusy = errors.New("node already has an active lease")
	// ErrLeaseNotFound is returned when a lease id is missing.
	ErrLeaseNotFound = errors.New("lease not found")
	// ErrLeaseExpired is returned when acting on an expired/released lease.
	ErrLeaseExpired = errors.New("lease expired or released")
)

// Lease is a short-lived reservation of a node for a task attempt. Unlike the
// original single-lease model, multiple active leases per node are now allowed
// up to the node's total concurrency (sum of Concurrency across all active
// capabilities). Released leases set Active=NULL; the index on (node_id, active)
// is no longer unique so concurrent tasks can share the node.
type Lease struct {
	Id      string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	NodeId  string `json:"node_id" gorm:"type:varchar(64);not null;index:idx_node_active_lease;index:idx_lease_node_category,priority:1"`
	TaskId  string `json:"task_id" gorm:"type:varchar(64);index;not null"`
	Attempt int    `json:"attempt" gorm:"not null"`
	// Active is NULL when released; non-NULL (true) while the lease is held.
	// The idx_node_active_lease index is NOT unique — multiple active leases per
	// node are allowed, bounded by the node's total concurrency capacity.
	Active   *bool  `json:"active" gorm:"index:idx_node_active_lease"`
	// ScriptId and Version track which script this lease is for, enabling the
	// per-script concurrency limit check during reservation.
	ScriptId int `json:"script_id" gorm:"index;default:0"`
	Version  int `json:"version" gorm:"default:0"`
	// CategoryId is the target-site category of the script, denormalized from the
	// script version at reservation time so the min-interval cooldown can be
	// enforced per (node, category) with a single-table lookup — no join. Zero for
	// uncategorized scripts (and for leases created before this column existed),
	// which are exempt from the cooldown.
	CategoryId int `json:"category_id" gorm:"index:idx_lease_node_category,priority:2;default:0"`
	ExpiresAt   int64  `json:"expires_at" gorm:"index;not null"`
	ReleasedAt  int64  `json:"released_at" gorm:"default:0"`
	Reason      string `json:"reason,omitempty" gorm:"type:varchar(64)"`
	LockVersion int64  `json:"lock_version" gorm:"default:0"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (Lease) TableName() string { return "leases" }

// TaskAttempt is one execution instance of an order's task. (task_id, attempt)
// is the execution idempotency key (architecture §8.3).
type TaskAttempt struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskId    string `json:"task_id" gorm:"type:varchar(64);index:idx_task_attempt,unique;not null"`
	OrderId   string `json:"order_id" gorm:"type:varchar(64);index;not null"`
	Attempt   int    `json:"attempt" gorm:"index:idx_task_attempt,unique;not null"`
	NodeId    string `json:"node_id" gorm:"type:varchar(64);index"`
	LeaseId   string `json:"lease_id" gorm:"type:varchar(64)"`
	State     string `json:"state" gorm:"type:varchar(16);default:RESERVED"`
	ErrorCode string `json:"error_code,omitempty" gorm:"type:varchar(48)"`
	// ScriptBalance is the account balance on the target site as reported by the
	// node plugin when the task completes. Nil means the plugin did not include a
	// balance in its result. Used by the settlement layer to update the capability's
	// remaining balance after a successful reconciliation.
	ScriptBalance *int  `json:"script_balance,omitempty" gorm:"column:script_balance"`
	CreatedAt     int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (TaskAttempt) TableName() string { return "task_attempts" }

func boolPtr(b bool) *bool { return &b }

// forUpdateOption returns the row-lock query option, empty on SQLite which does
// not support SELECT ... FOR UPDATE (tests run on SQLite).
func forUpdateOption() string {
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return ""
	}
	return "FOR UPDATE"
}

// nodeCapacityInTx returns the total concurrency capacity of a node (sum of
// Concurrency across all active capabilities) and the current active lease count.
// Both values are computed within the passed transaction.
func nodeCapacityInTx(tx *gorm.DB, nodeId string) (totalConcurrency int64, activeLeases int64, err error) {
	row := tx.Table("node_capabilities").
		Select("COALESCE(SUM(concurrency), 0)").
		Where("node_id = ? AND status = ?", nodeId, CapabilityStatusActive)
	if err = row.Scan(&totalConcurrency).Error; err != nil {
		return
	}
	if totalConcurrency < 1 {
		totalConcurrency = 1
	}
	err = tx.Model(&Lease{}).
		Where("node_id = ? AND active = ?", nodeId, true).
		Count(&activeLeases).Error
	return
}

// ReserveNode atomically creates an active lease for a node and a RESERVED task
// attempt. Unlike the old single-lease model, multiple leases are allowed up to
// the node's total concurrency (sum of capability.Concurrency). There is also a
// per-script limit: active leases for the same (node, script, version) must be
// below that capability's concurrency. ErrNodeBusy is returned when either
// limit is reached.
func ReserveNode(taskId, orderId, nodeId string, scriptId, version, attempt int, ttl time.Duration) (*Lease, *TaskAttempt, error) {
	var lease *Lease
	var ta *TaskAttempt
	err := DB.Transaction(func(tx *gorm.DB) error {
		var node Node
		if err := tx.Set("gorm:query_option", forUpdateOption()).Where("id = ?", nodeId).First(&node).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNodeNotFound
			}
			return err
		}
		if !node.IsOnline() {
			return ErrNodeBusy
		}
		totalCap, activeCount, err := nodeCapacityInTx(tx, nodeId)
		if err != nil {
			return err
		}
		if activeCount >= totalCap {
			return ErrNodeBusy
		}
		// Per-script concurrency guard: count active leases for this exact script.
		var scriptCap int64
		tx.Table("node_capabilities").
			Select("COALESCE(concurrency, 1)").
			Where("node_id = ? AND script_id = ? AND version = ? AND status = ?", nodeId, scriptId, version, CapabilityStatusActive).
			Scan(&scriptCap)
		if scriptCap < 1 {
			scriptCap = 1
		}
		var scriptActive int64
		tx.Model(&Lease{}).
			Where("node_id = ? AND script_id = ? AND version = ? AND active = ?", nodeId, scriptId, version, true).
			Count(&scriptActive)
		if scriptActive >= scriptCap {
			return ErrNodeBusy
		}

		l := &Lease{
			Id:        "lea_" + common.GetUUID(),
			NodeId:    nodeId,
			TaskId:    taskId,
			ScriptId:  scriptId,
			Version:   version,
			Attempt:   attempt,
			Active:    boolPtr(true),
			ExpiresAt: time.Now().Add(ttl).Unix(),
		}
		if err := tx.Create(l).Error; err != nil {
			return err
		}
		attemptRow := &TaskAttempt{
			TaskId: taskId, OrderId: orderId, Attempt: attempt,
			NodeId: nodeId, LeaseId: l.Id, State: AttemptReserved,
		}
		if err := tx.Create(attemptRow).Error; err != nil {
			if isUniqueConstraintErr(err) {
				return errors.New("task attempt already exists")
			}
			return err
		}
		// Flip to BUSY whenever any active lease exists (may already be BUSY).
		if err := tx.Model(&Node{}).Where("id = ?", nodeId).Update("state", NodeStateBusy).Error; err != nil {
			return err
		}
		lease, ta = l, attemptRow
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return lease, ta, nil
}

// RenewLease extends an active lease using optimistic locking.
func RenewLease(leaseId string, ttl time.Duration, expectedVersion int64) (*Lease, error) {
	var updated *Lease
	err := DB.Transaction(func(tx *gorm.DB) error {
		var l Lease
		if err := tx.Where("id = ?", leaseId).First(&l).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLeaseNotFound
			}
			return err
		}
		if l.Active == nil || !*l.Active {
			return ErrLeaseExpired
		}
		res := tx.Model(&Lease{}).
			Where("id = ? AND lock_version = ?", leaseId, expectedVersion).
			Updates(map[string]any{
				"expires_at":   time.Now().Add(ttl).Unix(),
				"lock_version": l.LockVersion + 1,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrOrderConcurrentUpdate
		}
		l.ExpiresAt = time.Now().Add(ttl).Unix()
		l.LockVersion++
		updated = &l
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ReleaseLease frees a node's active lease (setting Active=NULL) and returns
// the node to IDLE only when no other active leases remain. It is idempotent:
// releasing an already-released lease is a no-op success.
func ReleaseLease(leaseId, reason string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var l Lease
		if err := tx.Where("id = ?", leaseId).First(&l).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLeaseNotFound
			}
			return err
		}
		if l.Active == nil {
			return nil // already released
		}
		if err := tx.Model(&Lease{}).Where("id = ?", leaseId).Updates(map[string]any{
			"active":      gorm.Expr("NULL"),
			"released_at": common.GetTimestamp(),
			"reason":      reason,
		}).Error; err != nil {
			return err
		}
		// Return the node to IDLE only when this was the last active lease.
		// With concurrent execution a node may still hold other active leases
		// (for other scripts / tasks); only go back to IDLE when none remain.
		var remaining int64
		if err := tx.Model(&Lease{}).
			Where("node_id = ? AND active = ?", l.NodeId, true).
			Count(&remaining).Error; err != nil {
			return err
		}
		if remaining == 0 {
			return tx.Model(&Node{}).
				Where("id = ? AND state = ?", l.NodeId, NodeStateBusy).
				Update("state", NodeStateIdle).Error
		}
		return nil
	})
}

// NodeCategoryCoolingUntil reports whether a node is within the min-interval
// cooldown for a target-site category and, if so, the Unix timestamp at which it
// becomes dispatchable again.
//
// The cooldown throttles how often a NEW execution may be STARTED against a
// site, independent of concurrency: even when the node has a free concurrency
// slot, a fresh dispatch must wait until minIntervalSeconds have elapsed since
// the last dispatch for this (node, category). The anchor is the most recent
// lease's created_at REGARDLESS of whether it is still active — a finished task
// must not reset the clock, or the gap the target site requires is lost.
//
// categoryId 0 (uncategorized) or minIntervalSeconds <= 0 means no cooldown:
// returns (false, 0). The passed tx may be nil to use the default DB handle.
func NodeCategoryCoolingUntil(tx *gorm.DB, nodeId string, categoryId, minIntervalSeconds int) (cooling bool, readyAt int64) {
	if categoryId <= 0 || minIntervalSeconds <= 0 {
		return false, 0
	}
	db := tx
	if db == nil {
		db = DB
	}
	var lastCreatedAt int64
	db.Model(&Lease{}).
		Where("node_id = ? AND category_id = ?", nodeId, categoryId).
		Select("COALESCE(MAX(created_at), 0)").
		Scan(&lastCreatedAt)
	if lastCreatedAt == 0 {
		return false, 0
	}
	readyAt = lastCreatedAt + int64(minIntervalSeconds)
	if time.Now().Unix() < readyAt {
		return true, readyAt
	}
	return false, 0
}

// GetActiveLeaseForNode returns the node's active lease, or nil if none.
func GetActiveLeaseForNode(nodeId string) (*Lease, error) {
	var l Lease
	err := DB.Where("node_id = ? AND active = ?", nodeId, true).First(&l).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// GetTaskAttempt returns a specific attempt for a task, or nil if absent.
func GetTaskAttempt(taskId string, attempt int) (*TaskAttempt, error) {
	var ta TaskAttempt
	err := DB.Where("task_id = ? AND attempt = ?", taskId, attempt).First(&ta).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ta, nil
}

// FinalizeTaskAttempt records the terminal state of a task attempt (SUCCEEDED or
// FAILED) after reconciliation. It is idempotent: re-finalizing to the same
// state is a no-op and it never overwrites an already-terminal attempt. This is
// what lets per-node/per-script stats compute a real success rate.
func FinalizeTaskAttempt(taskId string, attempt int, state string) error {
	return DB.Model(&TaskAttempt{}).
		Where("task_id = ? AND attempt = ? AND state NOT IN ?",
			taskId, attempt, []string{AttemptSucceeded, AttemptFailed}).
		Updates(map[string]any{"state": state, "updated_at": time.Now().Unix()}).Error
}

// SetTaskAttemptBalance stores the script-reported account balance for a task
// attempt. Called when the node plugin includes a balance field in its
// task.result_ready message so the settlement layer can later update the
// capability's remaining balance. Idempotent: safe to call multiple times.
func SetTaskAttemptBalance(taskId string, attempt, balance int) error {
	return DB.Model(&TaskAttempt{}).
		Where("task_id = ? AND attempt = ?", taskId, attempt).
		Update("script_balance", balance).Error
}

// GetNode returns a node by id.
func GetNode(nodeId string) (*Node, error) {
	var n Node
	if err := DB.Where("id = ?", nodeId).First(&n).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}
	return &n, nil
}

// ExpireStaleLeases releases all active leases past their expiry, returning the
// count released. Intended to be called by a periodic job.
func ExpireStaleLeases(now int64) (int, error) {
	var stale []Lease
	if err := DB.Where("active = ? AND expires_at < ?", true, now).Find(&stale).Error; err != nil {
		return 0, err
	}
	released := 0
	for _, l := range stale {
		if err := ReleaseLease(l.Id, "expired"); err != nil {
			return released, err
		}
		released++
	}
	return released, nil
}
