package model

import (
	"errors"
	"strconv"
	"strings"
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
	Id       string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	DeviceId string `json:"device_id" gorm:"type:varchar(64);index;not null"`
	UserId   int    `json:"user_id" gorm:"index;not null"`
	// ProviderGroupId is the logical group this node belongs to (one group per
	// owning user, auto-created from the username). Clients can filter offers to
	// a single provider by this id.
	ProviderGroupId string `json:"provider_group_id" gorm:"type:varchar(64);index"`
	State           string `json:"state" gorm:"type:varchar(16);index;default:OFFLINE"`
	// Enabled is the provider's explicit on/off switch for scheduling. A node is
	// only a scheduling/offer candidate when Enabled is true (in addition to being
	// online). Default false: a freshly registered node stays out of the market
	// until its owner lists capabilities, passes their balance checks and turns it
	// on (PRD N-002 — provider opts in per node).
	Enabled         bool   `json:"enabled" gorm:"index;default:false"`
	Region          string `json:"region" gorm:"type:varchar(32)"`
	Version         string `json:"version" gorm:"type:varchar(32)"`
	LastSeenAt      int64  `json:"last_seen_at" gorm:"index;default:0"`
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

// IsValid reports whether the node has a passing balance check for the
// category. A passed check does not expire on a timer: it stays valid until an
// explicit recheck flips balance_ok to false. If the node is actually broken at
// execution time the order fails, which shows up in the success rate — that is
// the only signal needed, and periodic re-probing is too burdensome at scale.
func (s *NodeSiteStatus) IsValid() bool {
	return s.BalanceOk
}

// NodeCapability is a script version a node has enabled, with its price, daily
// limit, working window and test validity. Default is disabled: a node must
// explicitly enable each (PRD N-002).
//
// Balance model: RemainingQuota is the account balance on the target site as
// reported by the last successful script execution — it is NOT set by the
// provider at listing time. First-time listing defaults to 100. DailyLimit caps
// how many executions are dispatched per Beijing calendar day; DailyUsed counts
// today's executions and is reset automatically at midnight CST (UTC+8).
type NodeCapability struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	NodeId     string `json:"node_id" gorm:"type:varchar(64);index:idx_node_cap,unique;not null"`
	ScriptId   int    `json:"script_id" gorm:"index:idx_node_cap,unique;not null"`
	Version    int    `json:"version" gorm:"index:idx_node_cap,unique;not null"`
	CategoryId int    `json:"category_id" gorm:"index;default:0"` // denormalized from the script
	UserId     int    `json:"user_id" gorm:"index;not null"`
	// PriceMicros is deprecated — kept for backward-compat reads from old rows.
	// New listings use PriceMultiplier × ScriptVersion.BasePriceMicros instead.
	PriceMicros    int64   `json:"price_micros,omitempty" gorm:"default:0"`
	// PriceMultiplier is the provider's markup on the script's base price.
	// Range: 0.5–10. Default 1.0 (pass-through).
	PriceMultiplier float64 `json:"price_multiplier" gorm:"default:1.0;not null"`
	// MinIntervalSeconds is denormalized from ScriptVersion.MinIntervalSeconds
	// so the scheduler can enforce the gap without an extra join.
	MinIntervalSeconds int `json:"min_interval_seconds" gorm:"default:30;not null"`
	// Concurrency is the maximum simultaneous executions for this script on this
	// node — denormalized from ScriptVersion.Concurrency at listing time so the
	// scheduler can compute per-node total capacity without joining versions.
	Concurrency int `json:"concurrency" gorm:"default:1;not null"`
	// DailyLimit is the maximum script executions dispatched per day.
	// Zero means no daily cap. Resets at midnight Beijing time (CST, UTC+8).
	DailyLimit int `json:"daily_limit" gorm:"column:daily_quota;default:0"`
	// DailyUsed counts successful executions since the last Beijing midnight.
	DailyUsed int `json:"daily_used" gorm:"column:daily_used;default:0"`
	// DailyResetAt is the Unix timestamp (seconds) of the Beijing midnight at
	// which DailyUsed was last zeroed. Zero means it has never been reset.
	DailyResetAt int64 `json:"daily_reset_at" gorm:"column:daily_reset_at;default:0"`
	// RemainingQuota is the target-site account balance as last reported by a
	// successful execution. Updated from the script result; defaults to 100 on
	// first listing.
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

// UpsertNode registers or updates a node for a device, refreshing presence. The
// node is placed in the owning user's provider group (created on first use) so
// every node has a group from its first heartbeat.
func UpsertNode(userId int, deviceId, nodeId, region, version string) (*Node, error) {
	now := time.Now().Unix()
	groupId := ensureUserProviderGroup(userId)
	var node Node
	err := DB.Where("id = ?", nodeId).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		node = Node{
			Id: nodeId, DeviceId: deviceId, UserId: userId, ProviderGroupId: groupId,
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
	updates := map[string]any{
		"region": region, "version": version, "last_seen_at": now, "state": node.State,
	}
	// Backfill the group on pre-existing nodes that predate grouping.
	if node.ProviderGroupId == "" && groupId != "" {
		updates["provider_group_id"] = groupId
		node.ProviderGroupId = groupId
	}
	if err := DB.Model(&Node{}).Where("id = ?", nodeId).Updates(updates).Error; err != nil {
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
	return DB.Transaction(func(tx *gorm.DB) error {
		var lockedNode Node
		if err := tx.Set("gorm:query_option", forUpdateOption()).Where("id = ?", nodeId).First(&lockedNode).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNodeNotFound
			}
			return err
		}

		var activeLeaseCount int64
		if err := tx.Model(&Lease{}).
			Where("node_id = ? AND active = ?", nodeId, true).
			Count(&activeLeaseCount).Error; err != nil {
			return err
		}
		// The active lease is the scheduling fact source. A plugin may still
		// report IDLE while executing, but it must not overwrite BUSY until the
		// lease has been released.
		if activeLeaseCount > 0 {
			state = NodeStateBusy
		}

		return tx.Model(&Node{}).Where("id = ?", nodeId).Updates(map[string]any{
			"last_seen_at": time.Now().Unix(),
			"state":        state,
		}).Error
	})
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
	if err := DB.Model(&NodeSiteStatus{}).Where("id = ?", existing.Id).Updates(map[string]any{
		"balance_ok": s.BalanceOk, "balance_micros": s.BalanceMicros,
		"tier": s.Tier, "checked_at": s.CheckedAt, "expires_at": s.ExpiresAt,
		"error_message": s.ErrorMessage,
	}).Error; err != nil {
		return err
	}
	// A balance check is the freshest reading of the provider's on-site account
	// balance, so mirror it onto every capability in this category. The scheduler
	// gates dispatch on cap.remaining_quota (RemainingQuota > consumeMultiplier)
	// and the console shows the same column, so without this write both would
	// keep using the stale seed/last-execution value instead of the just-probed
	// balance. Only a passing check overwrites it — a failed probe must not zero
	// out a known-good balance.
	if s.BalanceOk {
		syncCapabilityQuotaToBalance(s.NodeId, s.CategoryId, s.BalanceMicros)
	}
	return nil
}

// syncCapabilityQuotaToBalance copies a category's freshly probed balance onto
// the RemainingQuota of every capability the node lists under that category, so
// the scheduler gate and the console balance column read the same number. Errors
// are non-fatal: the balance check itself already succeeded, and the scheduler
// falls back to the previous quota until the next probe.
func syncCapabilityQuotaToBalance(nodeId string, categoryId int, balanceMicros int64) {
	if categoryId <= 0 {
		return
	}
	_ = DB.Model(&NodeCapability{}).
		Where("node_id = ? AND category_id = ?", nodeId, categoryId).
		UpdateColumn("remaining_quota", balanceMicros).Error
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

// EnableCapability lists (or updates) a script version on a node. It requires
// the referenced version to be executable and a valid challenge-test window.
//
// Listing no longer gates on a passing balance check: a provider lists the
// script first, then runs the per-capability balance check, and finally turns
// the node on (SetNodeEnabled) once all its capabilities pass. The category and
// concurrency are denormalized here so the scheduler can compute node capacity
// without joining script_versions on every scheduling query.
//
// On first listing RemainingQuota is set to 100 as the initial balance estimate;
// subsequent executions update it from the actual script result. Re-listing an
// existing capability preserves the last-known balance and daily counters so
// the provider's execution history is not wiped on config changes.
func EnableCapability(cap *NodeCapability) error {
	sv, err := GetExecutableScriptVersion(cap.ScriptId, cap.Version)
	if err != nil {
		return err
	}
	if !cap.IsTestValid() {
		return ErrCapabilityTestRequired
	}
	// Denormalize category, concurrency and interval from the script version so
	// scheduling queries can compute node capacity without an extra join.
	cap.CategoryId = sv.CategoryId
	concurrency := sv.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	cap.Concurrency = concurrency
	cap.MinIntervalSeconds = sv.MinIntervalSeconds
	// Compute the effective price_micros cache: base × multiplier.
	// The scheduler reads this column directly so it never needs a join.
	if sv.BasePriceMicros > 0 {
		cap.PriceMicros = int64(float64(sv.BasePriceMicros) * cap.PriceMultiplier)
	}
	cap.Status = CapabilityStatusActive
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing NodeCapability
		err := tx.Where("node_id = ? AND script_id = ? AND version = ?", cap.NodeId, cap.ScriptId, cap.Version).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// First listing: seed balance with a conservative default of 100.
			cap.RemainingQuota = 100
			return tx.Create(cap).Error
		}
		if err != nil {
			return err
		}
		cap.Id = existing.Id
		// Re-listing: update listing metadata including the latest concurrency.
		// Preserve the existing balance and daily counters so history is kept.
		return tx.Model(&NodeCapability{}).Where("id = ?", existing.Id).Updates(map[string]any{
			"category_id":          cap.CategoryId,
			"concurrency":          cap.Concurrency,
			"min_interval_seconds": cap.MinIntervalSeconds,
			"price_multiplier":     cap.PriceMultiplier,
			"price_micros":         cap.PriceMicros, // cached: base × multiplier
			"daily_quota":          cap.DailyLimit,
			"work_window":          cap.WorkWindow,
			"status":               CapabilityStatusActive,
			"suspend_reason":       "",
			"test_expires_at":      cap.TestExpiresAt,
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

// beijingMidnightUnix returns the Unix timestamp (seconds) of the start of the
// current calendar day in Beijing time (CST, UTC+8). Used to detect day rollover
// for daily execution counter resets.
func beijingMidnightUnix() int64 {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// Fallback: UTC+8 fixed offset if the timezone database is unavailable.
		loc = time.FixedZone("CST", 8*3600)
	}
	now := time.Now().In(loc)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return midnight.Unix()
}

// UpdateCapabilityBalance sets RemainingQuota to the balance value reported by a
// successful script execution. Called by the settlement layer after reconciling
// a succeeded task; the script result carries the current account balance on the
// target site. Only updates if balance > 0 to avoid overwriting with a zero
// returned by a script that doesn't report balance.
func UpdateCapabilityBalance(nodeId string, scriptId, version, balance int) error {
	if balance <= 0 {
		return nil
	}
	return DB.Model(&NodeCapability{}).
		Where("node_id = ? AND script_id = ? AND version = ?", nodeId, scriptId, version).
		UpdateColumn("remaining_quota", balance).Error
}

// IncrementDailyUsed increments the today-execution counter for a capability.
// If the current Beijing calendar day has advanced since the last reset
// (daily_reset_at < today midnight), it zeroes the counter first and records
// the new day boundary. Safe to call on every successful settlement; the CASE
// expression makes the reset+increment atomic in a single UPDATE statement.
func IncrementDailyUsed(nodeId string, scriptId, version int) error {
	today := beijingMidnightUnix()
	return DB.Model(&NodeCapability{}).
		Where("node_id = ? AND script_id = ? AND version = ?", nodeId, scriptId, version).
		Updates(map[string]any{
			"daily_used": gorm.Expr(
				"CASE WHEN daily_reset_at < ? THEN 1 ELSE daily_used + 1 END", today,
			),
			"daily_reset_at": gorm.Expr(
				"CASE WHEN daily_reset_at < ? THEN ? ELSE daily_reset_at END", today, today,
			),
		}).Error
}

// ListNodeCapabilities returns capabilities for a node.
func ListNodeCapabilities(nodeId string) ([]NodeCapability, error) {
	var caps []NodeCapability
	err := DB.Where("node_id = ?", nodeId).Order("id desc").Find(&caps).Error
	return caps, err
}

// ProviderCapabilityStat is the per-(node, script version) execution summary for
// a provider: how many task attempts ran on that capability, how many settled
// successfully, and the gross provider revenue those successes earned.
type ProviderCapabilityStat struct {
	NodeId        string `json:"node_id"`
	ScriptId      int    `json:"script_id"`
	Version       int    `json:"version"`
	Executions    int64  `json:"executions"`
	Successes     int64  `json:"successes"`
	RevenueMicros int64  `json:"revenue_micros"`
}

// GetProviderCapabilityStats returns per-(node, script, version) execution stats
// for every node owned by userId, derived from task attempts joined to their
// orders and price snapshots. Only finalized attempts contribute a success or
// revenue; a RESERVED/RUNNING attempt still counts as an execution. Revenue is
// the sum of the frozen provider amount over succeeded attempts.
func GetProviderCapabilityStats(userId int) ([]ProviderCapabilityStat, error) {
	var stats []ProviderCapabilityStat
	err := DB.Table("task_attempts AS ta").
		Select(`ta.node_id AS node_id,
			o.script_id AS script_id,
			o.version AS version,
			COUNT(*) AS executions,
			SUM(CASE WHEN ta.state = ? THEN 1 ELSE 0 END) AS successes,
			COALESCE(SUM(CASE WHEN ta.state = ? THEN ps.provider_amount_micros ELSE 0 END), 0) AS revenue_micros`,
			AttemptSucceeded, AttemptSucceeded).
		Joins("JOIN orders o ON o.id = ta.order_id").
		Joins("LEFT JOIN order_price_snapshots ps ON ps.order_id = o.id").
		Where("ta.node_id IN (?)", DB.Table("nodes").Select("id").Where("user_id = ?", userId)).
		Group("ta.node_id, o.script_id, o.version").
		Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// ProviderTaskAttempt is one execution record on a provider's node, joined to
// its order so the provider sees which script/version ran and how it ended. The
// task params/result are never here — those travel the E2EE data plane and are
// only hashed on the control plane — so this exposes the most the server can
// know: the attempt state, failure code, target-site balance the plugin
// reported, and timing. Providers use it to debug their nodes' behavior.
type ProviderTaskAttempt struct {
	TaskId    string `json:"task_id"`
	OrderId   string `json:"order_id"`
	NodeId    string `json:"node_id"`
	Attempt   int    `json:"attempt"`
	State     string `json:"state"`
	ErrorCode string `json:"error_code,omitempty"`
	// ScriptBalance is the target-site account balance the plugin reported on a
	// successful run; nil when the plugin did not include one.
	ScriptBalance *int   `json:"script_balance,omitempty"`
	ScriptId      int    `json:"script_id"`
	Version       int    `json:"version"`
	InputHash     string `json:"input_hash,omitempty"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// ListProviderTaskAttempts returns the most recent task attempts across every
// node owned by userId (newest first), joined to their orders for the script
// version context. limit is clamped to [1,500] (default 100 when <= 0); offset
// defaults to 0. Only reads existing rows — no p2p/plugin change.
func ListProviderTaskAttempts(userId, limit, offset int) ([]ProviderTaskAttempt, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	var attempts []ProviderTaskAttempt
	err := DB.Table("task_attempts AS ta").
		Select(`ta.task_id AS task_id,
			ta.order_id AS order_id,
			ta.node_id AS node_id,
			ta.attempt AS attempt,
			ta.state AS state,
			ta.error_code AS error_code,
			ta.script_balance AS script_balance,
			o.script_id AS script_id,
			o.version AS version,
			o.input_hash AS input_hash,
			ta.created_at AS created_at,
			ta.updated_at AS updated_at`).
		Joins("JOIN orders o ON o.id = ta.order_id").
		Where("ta.node_id IN (?)", DB.Table("nodes").Select("id").Where("user_id = ?", userId)).
		Order("ta.id desc").
		Limit(limit).
		Offset(offset).
		Scan(&attempts).Error
	if err != nil {
		return nil, err
	}
	return attempts, nil
}

// ListNodesByUser returns all nodes owned by a user, newest heartbeat first.
func ListNodesByUser(userId int) ([]Node, error) {
	var nodes []Node
	err := DB.Where("user_id = ?", userId).Order("last_seen_at desc").Find(&nodes).Error
	return nodes, err
}

// ErrNodeEnableBlocked is returned when turning a node on while one or more of
// its active capabilities has not passed the (unexpired) balance check for its
// category. The message lists the offending capabilities so the UI can point
// the provider at what still needs checking.
type ErrNodeEnableBlocked struct{ Message string }

func (e *ErrNodeEnableBlocked) Error() string { return e.Message }

// ErrNodeNoCapabilities is returned when enabling a node that has no active
// capability to schedule — turning it on would put nothing on the market.
var ErrNodeNoCapabilities = errors.New("node has no listed capability to enable")

// SetNodeEnabled flips a node's scheduling switch. Turning it on requires every
// active capability that targets a site category to have a passing, unexpired
// balance check (the provider proved each target site is usable). Turning it
// off is unconditional. Cross-checks ownership.
func SetNodeEnabled(userId int, nodeId string, enabled bool) error {
	var node Node
	if err := DB.Where("id = ? AND user_id = ?", nodeId, userId).First(&node).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNodeNotFound
		}
		return err
	}
	if enabled {
		var caps []NodeCapability
		if err := DB.Where("node_id = ? AND status = ?", nodeId, CapabilityStatusActive).Find(&caps).Error; err != nil {
			return err
		}
		if len(caps) == 0 {
			return ErrNodeNoCapabilities
		}
		// Collect the distinct categories that still lack a valid balance check.
		blocked := map[int]bool{}
		for _, cap := range caps {
			if cap.CategoryId <= 0 || blocked[cap.CategoryId] {
				continue
			}
			ok, err := HasValidBalanceCheck(nodeId, cap.CategoryId)
			if err != nil {
				return err
			}
			if !ok {
				blocked[cap.CategoryId] = true
			}
		}
		if len(blocked) > 0 {
			ids := make([]int, 0, len(blocked))
			for id := range blocked {
				ids = append(ids, id)
			}
			names := categoryNames(ids)
			return &ErrNodeEnableBlocked{
				Message: "these categories still need a passing balance check before enabling: " + strings.Join(names, ", "),
			}
		}
	}
	return DB.Model(&Node{}).Where("id = ?", nodeId).Update("enabled", enabled).Error
}

// categoryNames resolves category ids to display names for error messages,
// falling back to the numeric id when a name is unavailable.
func categoryNames(ids []int) []string {
	var cats []ScriptCategory
	_ = DB.Where("id IN ?", ids).Find(&cats).Error
	byId := make(map[int]string, len(cats))
	for _, cat := range cats {
		byId[cat.Id] = cat.Name
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := byId[id]; ok && name != "" {
			names = append(names, name)
		} else {
			names = append(names, "#"+strconv.Itoa(id))
		}
	}
	return names
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
