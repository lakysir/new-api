package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Node states mirror the node runtime state machine (architecture §6.2). The
// authoritative online/offline judgement is derived from LastSeenAt + timeout;
// State is the coarse lifecycle marker.
const (
	NodeStateOffline  = "OFFLINE"
	NodeStateIdle     = "IDLE"
	NodeStateBusy     = "BUSY"
	NodeStateDraining = "DRAINING"
)

// Capability states.
const (
	CapabilityStatusActive    = "active"
	CapabilityStatusSuspended = "suspended"
)

// NodePresenceTimeout is the offline cutoff (PRD §9: last_seen + 45s).
const NodePresenceTimeout = 45 * time.Second

var (
	// ErrNodeNotFound is returned when a node row is missing.
	ErrNodeNotFound = errors.New("node not found")
	// ErrCapabilityTestRequired is returned when enabling a capability without a
	// valid challenge test.
	ErrCapabilityTestRequired = errors.New("capability requires a passing test before listing")
)

// Node is a Provider execution endpoint bound to a device. One device may run
// one node in the MVP.
type Node struct {
	Id         string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	DeviceId   string `json:"device_id" gorm:"type:varchar(64);index;not null"`
	UserId     int    `json:"user_id" gorm:"index;not null"`
	State      string `json:"state" gorm:"type:varchar(16);index;default:OFFLINE"`
	Region     string `json:"region" gorm:"type:varchar(32)"`
	Version    string `json:"version" gorm:"type:varchar(32)"`
	LastSeenAt int64  `json:"last_seen_at" gorm:"index;default:0"`
	// Execution outcome counters drive the scheduler's success-rate ranking.
	SuccessCount int64 `json:"success_count" gorm:"default:0"`
	FailureCount int64 `json:"failure_count" gorm:"default:0"`
	CreatedAt    int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (Node) TableName() string { return "nodes" }

// SuccessRate returns a Laplace-smoothed success rate in [0,1]:
// (success + 1) / (success + failure + 2). Smoothing gives a new node a neutral
// 0.5 so it can earn work, instead of being stuck at 0 forever.
func (n *Node) SuccessRate() float64 {
	return float64(n.SuccessCount+1) / float64(n.SuccessCount+n.FailureCount+2)
}

// RecordTaskOutcome increments a node's success or failure counter after a task
// completes. Higher success rate ranks a node higher in scheduling; a failed
// call lowers it, a successful one raises it.
func RecordTaskOutcome(nodeId string, success bool) error {
	col := "failure_count"
	if success {
		col = "success_count"
	}
	res := DB.Model(&Node{}).Where("id = ?", nodeId).
		UpdateColumn(col, gorm.Expr(col+" + 1"))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// IsOnline reports whether the node's last heartbeat is within the timeout.
func (n *Node) IsOnline() bool {
	return n.State != NodeStateOffline && n.LastSeenAt >= time.Now().Add(-NodePresenceTimeout).Unix()
}

// NodeSiteStatus records the result of a node's balance-probe for one category
// (target site). A node may only list capabilities in a category whose probe
// has passed and is unexpired — this is how "read balance to check the site is
// usable" gates going online, without running a generation task.
type NodeSiteStatus struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	NodeId        string `json:"node_id" gorm:"type:varchar(64);index:idx_node_site,unique;not null"`
	CategoryId    int    `json:"category_id" gorm:"index:idx_node_site,unique;not null"`
	UserId        int    `json:"user_id" gorm:"index;not null"`
	BalanceOk     bool   `json:"balance_ok" gorm:"index"`
	BalanceMicros int64  `json:"balance_micros" gorm:"default:0"` // reported balance (informational)
	Tier          string `json:"tier" gorm:"type:varchar(32)"`    // reported account tier
	ErrorMessage  string `json:"error_message,omitempty" gorm:"type:varchar(512)"`
	CheckedAt     int64  `json:"checked_at" gorm:"default:0"`
	ExpiresAt     int64  `json:"expires_at" gorm:"index;default:0"`
}

func (NodeSiteStatus) TableName() string { return "node_site_status" }

// IsValid reports whether a passed balance check is still within its window.
func (s *NodeSiteStatus) IsValid() bool {
	return s.BalanceOk && s.ExpiresAt > time.Now().Unix()
}

// NodeCapability is a script version a node has enabled, with its price, quota,
// working window and test validity. Default is disabled: a node must explicitly
// enable each (PRD N-002).
type NodeCapability struct {
	Id             int    `json:"id" gorm:"primaryKey;autoIncrement"`
	NodeId         string `json:"node_id" gorm:"type:varchar(64);index:idx_node_cap,unique;not null"`
	ScriptId       int    `json:"script_id" gorm:"index:idx_node_cap,unique;not null"`
	Version        int    `json:"version" gorm:"index:idx_node_cap,unique;not null"`
	CategoryId     int    `json:"category_id" gorm:"index;default:0"` // denormalized from the script
	UserId         int    `json:"user_id" gorm:"index;not null"`
	PriceMicros    int64  `json:"price_micros" gorm:"default:0"`
	DailyQuota     int    `json:"daily_quota" gorm:"default:0"`
	RemainingQuota int    `json:"remaining_quota" gorm:"default:0"`
	WorkWindow     string `json:"work_window" gorm:"type:varchar(64)"`
	Status         string `json:"status" gorm:"type:varchar(16);index;default:suspended"`
	SuspendReason  string `json:"suspend_reason,omitempty" gorm:"type:varchar(128)"`
	TestExpiresAt  int64  `json:"test_expires_at" gorm:"default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (NodeCapability) TableName() string { return "node_capabilities" }

// IsTestValid reports whether the capability's challenge test is unexpired.
func (cap *NodeCapability) IsTestValid() bool {
	return cap.TestExpiresAt > time.Now().Unix()
}

// UpsertNode registers or updates a node for a device, refreshing presence.
func UpsertNode(userId int, deviceId, nodeId, region, version string) (*Node, error) {
	now := time.Now().Unix()
	var node Node
	err := DB.Where("id = ?", nodeId).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		node = Node{
			Id: nodeId, DeviceId: deviceId, UserId: userId,
			State: NodeStateIdle, Region: region, Version: version, LastSeenAt: now,
		}
		if err := DB.Create(&node).Error; err != nil {
			return nil, err
		}
		return &node, nil
	}
	if err != nil {
		return nil, err
	}
	node.Region, node.Version, node.LastSeenAt = region, version, now
	if node.State == NodeStateOffline {
		node.State = NodeStateIdle
	}
	if err := DB.Model(&Node{}).Where("id = ?", nodeId).Updates(map[string]any{
		"region": region, "version": version, "last_seen_at": now, "state": node.State,
	}).Error; err != nil {
		return nil, err
	}
	return &node, nil
}

// TouchNodePresence records a heartbeat and idle/busy state for a node.
// ErrNodeDeviceRevoked is returned when a heartbeat arrives for a node whose
// owning device has been revoked — the caller (WSS handler) must drop the
// connection so a zombie socket can't resurrect a revoked node to IDLE.
var ErrNodeDeviceRevoked = errors.New("node's device is revoked")

func TouchNodePresence(nodeId, state string) error {
	// Refuse to refresh presence for a node whose device is revoked; otherwise a
	// still-open WSS connection would keep the node online after revocation.
	var node Node
	if err := DB.Where("id = ?", nodeId).First(&node).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNodeNotFound
		}
		return err
	}
	var device Device
	if err := DB.Select("status").Where("id = ?", node.DeviceId).First(&device).Error; err == nil {
		if device.Status == DeviceStatusRevoked {
			// Keep the node offline regardless of the incoming heartbeat.
			DB.Model(&Node{}).Where("id = ?", nodeId).Update("state", NodeStateOffline)
			return ErrNodeDeviceRevoked
		}
	}
	res := DB.Model(&Node{}).Where("id = ?", nodeId).Updates(map[string]any{
		"last_seen_at": time.Now().Unix(),
		"state":        state,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// ErrBalanceCheckRequired is returned when a node has no valid balance-probe
// for the script's category — it must read the site balance first.
var ErrBalanceCheckRequired = errors.New("node must pass the category balance check before listing")

// RecordBalanceCheck upserts a node's balance-probe result for a category. A
// successful probe (balance readable) grants a window during which the node may
// list capabilities in that category. Called after the plugin runs the
// category's balance-probe script.
func RecordBalanceCheck(s *NodeSiteStatus) error {
	var existing NodeSiteStatus
	err := DB.Where("node_id = ? AND category_id = ?", s.NodeId, s.CategoryId).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DB.Create(s).Error
	}
	if err != nil {
		return err
	}
	return DB.Model(&NodeSiteStatus{}).Where("id = ?", existing.Id).Updates(map[string]any{
		"balance_ok": s.BalanceOk, "balance_micros": s.BalanceMicros,
		"tier": s.Tier, "checked_at": s.CheckedAt, "expires_at": s.ExpiresAt,
		"error_message": s.ErrorMessage,
	}).Error
}

// HasValidBalanceCheck reports whether a node has a passing, unexpired probe for
// a category.
func HasValidBalanceCheck(nodeId string, categoryId int) (bool, error) {
	var s NodeSiteStatus
	err := DB.Where("node_id = ? AND category_id = ?", nodeId, categoryId).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.IsValid(), nil
}

// ListNodeSiteStatuses returns the latest balance probe for every category on
// a node. Probe execution is free and therefore has no order or ledger rows.
func ListNodeSiteStatuses(nodeId string) ([]NodeSiteStatus, error) {
	var statuses []NodeSiteStatus
	err := DB.Where("node_id = ?", nodeId).Order("category_id ASC").Find(&statuses).Error
	return statuses, err
}

// EnableCapability lists (or updates) a script version on a node. It requires:
// the referenced version to be executable, a valid challenge-test window, AND a
// passing balance check for the script's category (the node proved the target
// site is usable by reading its balance, not by running a generation).
func EnableCapability(cap *NodeCapability) error {
	sv, err := GetExecutableScriptVersion(cap.ScriptId, cap.Version)
	if err != nil {
		return err
	}
	if !cap.IsTestValid() {
		return ErrCapabilityTestRequired
	}
	// Denormalize the category and gate on its balance check.
	cap.CategoryId = sv.CategoryId
	if cap.CategoryId > 0 {
		ok, err := HasValidBalanceCheck(cap.NodeId, cap.CategoryId)
		if err != nil {
			return err
		}
		if !ok {
			return ErrBalanceCheckRequired
		}
	}
	cap.Status = CapabilityStatusActive
	if cap.RemainingQuota == 0 {
		cap.RemainingQuota = cap.DailyQuota
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing NodeCapability
		err := tx.Where("node_id = ? AND script_id = ? AND version = ?", cap.NodeId, cap.ScriptId, cap.Version).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(cap).Error
		}
		if err != nil {
			return err
		}
		cap.Id = existing.Id
		return tx.Model(&NodeCapability{}).Where("id = ?", existing.Id).Updates(map[string]any{
			"category_id":     cap.CategoryId,
			"price_micros":    cap.PriceMicros,
			"daily_quota":     cap.DailyQuota,
			"remaining_quota": cap.RemainingQuota,
			"work_window":     cap.WorkWindow,
			"status":          CapabilityStatusActive,
			"suspend_reason":  "",
			"test_expires_at": cap.TestExpiresAt,
		}).Error
	})
}

// RemoveCapability takes a provider capability off the market. Removing the
// row lets the same script version be listed again later as a fresh capability.
func RemoveCapability(userId int, nodeId string, scriptId, version int) error {
	res := DB.
		Where("node_id = ? AND script_id = ? AND version = ? AND user_id = ?", nodeId, scriptId, version, userId).
		Delete(&NodeCapability{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("capability not found")
	}
	return nil
}

// ConsumeCapabilityQuota decrements one successful execution from the exact
// capability used by a settled task. The guarded update prevents underflow.
func ConsumeCapabilityQuota(nodeId string, scriptId, version int) error {
	return DB.Model(&NodeCapability{}).
		Where("node_id = ? AND script_id = ? AND version = ? AND remaining_quota > 0", nodeId, scriptId, version).
		UpdateColumn("remaining_quota", gorm.Expr("remaining_quota - 1")).Error
}

// ListNodeCapabilities returns capabilities for a node.
func ListNodeCapabilities(nodeId string) ([]NodeCapability, error) {
	var caps []NodeCapability
	err := DB.Where("node_id = ?", nodeId).Order("id desc").Find(&caps).Error
	return caps, err
}

// ListNodesByUser returns all nodes owned by a user, newest heartbeat first.
func ListNodesByUser(userId int) ([]Node, error) {
	var nodes []Node
	err := DB.Where("user_id = ?", userId).Order("last_seen_at desc").Find(&nodes).Error
	return nodes, err
}

// ErrNodeStillOnline is returned when deleting a node that is still online.
var ErrNodeStillOnline = errors.New("only offline nodes can be deleted")

// DeleteOfflineNode hard-deletes an offline node and its capabilities. Refuses
// an online node (must go offline first). Cross-checks ownership.
func DeleteOfflineNode(userId int, nodeId string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var n Node
		if err := tx.Where("id = ? AND user_id = ?", nodeId, userId).First(&n).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNodeNotFound
			}
			return err
		}
		if n.IsOnline() {
			return ErrNodeStillOnline
		}
		if err := tx.Where("node_id = ?", nodeId).Delete(&NodeCapability{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", nodeId).Delete(&Node{}).Error
	})
}

// suspendCapabilitiesByDeviceTx suspends all capabilities of a device's nodes
// within an existing transaction (used on device revocation).
func suspendCapabilitiesByDeviceTx(tx *gorm.DB, deviceId, reason string) error {
	var nodeIds []string
	if err := tx.Model(&Node{}).Where("device_id = ?", deviceId).Pluck("id", &nodeIds).Error; err != nil {
		return err
	}
	if len(nodeIds) == 0 {
		return nil
	}
	return tx.Model(&NodeCapability{}).Where("node_id IN ?", nodeIds).
		Updates(map[string]any{"status": CapabilityStatusSuspended, "suspend_reason": reason}).Error
}

// SuspendCapabilitiesByScriptVersion suspends every capability bound to a
// revoked script version across all nodes (PRD N-007, architecture §22.3).
func SuspendCapabilitiesByScriptVersion(scriptId, version int) (int64, error) {
	res := DB.Model(&NodeCapability{}).
		Where("script_id = ? AND version = ? AND status = ?", scriptId, version, CapabilityStatusActive).
		Updates(map[string]any{"status": CapabilityStatusSuspended, "suspend_reason": "script_revoked"})
	return res.RowsAffected, res.Error
}
