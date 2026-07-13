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
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (Node) TableName() string { return "nodes" }

// IsOnline reports whether the node's last heartbeat is within the timeout.
func (n *Node) IsOnline() bool {
	return n.State != NodeStateOffline && n.LastSeenAt >= time.Now().Add(-NodePresenceTimeout).Unix()
}

// NodeCapability is a script version a node has enabled, with its price, quota,
// working window and test validity. Default is disabled: a node must explicitly
// enable each (PRD N-002).
type NodeCapability struct {
	Id             int    `json:"id" gorm:"primaryKey;autoIncrement"`
	NodeId         string `json:"node_id" gorm:"type:varchar(64);index:idx_node_cap,unique;not null"`
	ScriptId       int    `json:"script_id" gorm:"index:idx_node_cap,unique;not null"`
	Version        int    `json:"version" gorm:"index:idx_node_cap,unique;not null"`
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
func TouchNodePresence(nodeId, state string) error {
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

// EnableCapability lists (or updates) a script version on a node. It requires
// the referenced script version to be executable (approved + not revoked) and a
// valid, unexpired test window (PRD N-003).
func EnableCapability(cap *NodeCapability) error {
	if _, err := GetExecutableScriptVersion(cap.ScriptId, cap.Version); err != nil {
		return err
	}
	if !cap.IsTestValid() {
		return ErrCapabilityTestRequired
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

// DisableCapability takes a capability off the market (Provider-initiated).
func DisableCapability(userId int, nodeId string, scriptId, version int) error {
	res := DB.Model(&NodeCapability{}).
		Where("node_id = ? AND script_id = ? AND version = ? AND user_id = ?", nodeId, scriptId, version, userId).
		Updates(map[string]any{"status": CapabilityStatusSuspended, "suspend_reason": "provider_disabled"})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("capability not found")
	}
	return nil
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
