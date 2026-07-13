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

// Lease is a short-lived exclusive reservation of a node for a task attempt.
// Active is a nullable bool used with a partial unique index so only one active
// lease per node can exist; released leases set Active=NULL to free the slot.
type Lease struct {
	Id      string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	NodeId  string `json:"node_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_node_active_lease"`
	TaskId  string `json:"task_id" gorm:"type:varchar(64);index;not null"`
	Attempt int    `json:"attempt" gorm:"not null"`
	// Active is part of a composite unique index with NodeId: at most one
	// (node_id, active=true) row can exist, so a node holds one active lease.
	// Released leases set Active=NULL; NULLs don't collide, freeing the slot.
	Active      *bool  `json:"active" gorm:"uniqueIndex:idx_node_active_lease"`
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
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64  `json:"updated_at" gorm:"autoUpdateTime"`
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

// ReserveNode atomically creates an active lease for a node and a RESERVED task
// attempt, but only if the node is currently IDLE and online and holds no other
// active lease. The unique index on (node_id) WHERE active guarantees that even
// under a race, only one caller wins; the loser gets ErrNodeBusy.
func ReserveNode(taskId, orderId, nodeId string, attempt int, ttl time.Duration) (*Lease, *TaskAttempt, error) {
	var lease *Lease
	var ta *TaskAttempt
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Lock the node row and re-check it is idle + online. FOR UPDATE matches
		// the codebase convention; the partial unique index on (node_id) WHERE
		// active is the real correctness guard, this only reduces contention.
		var node Node
		if err := tx.Set("gorm:query_option", forUpdateOption()).Where("id = ?", nodeId).First(&node).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNodeNotFound
			}
			return err
		}
		if node.State != NodeStateIdle || !node.IsOnline() {
			return ErrNodeBusy
		}

		l := &Lease{
			Id:        "lea_" + common.GetUUID(),
			NodeId:    nodeId,
			TaskId:    taskId,
			Attempt:   attempt,
			Active:    boolPtr(true),
			ExpiresAt: time.Now().Add(ttl).Unix(),
		}
		if err := tx.Create(l).Error; err != nil {
			if isUniqueConstraintErr(err) {
				return ErrNodeBusy // another active lease exists for this node
			}
			return err
		}

		attemptRow := &TaskAttempt{
			TaskId: taskId, OrderId: orderId, Attempt: attempt,
			NodeId: nodeId, LeaseId: l.Id, State: AttemptReserved,
		}
		if err := tx.Create(attemptRow).Error; err != nil {
			if isUniqueConstraintErr(err) {
				return errors.New("task attempt already exists") // (task_id, attempt) reused
			}
			return err
		}

		// Flip the node to BUSY so scheduling excludes it immediately.
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

// ReleaseLease frees a node's active lease (setting Active=NULL so the partial
// unique index no longer blocks a new lease) and returns the node to IDLE. It
// is idempotent: releasing an already-released lease is a no-op success.
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
		// Only return the node to IDLE if it is not offline/draining.
		return tx.Model(&Node{}).
			Where("id = ? AND state = ?", l.NodeId, NodeStateBusy).
			Update("state", NodeStateIdle).Error
	})
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
