package model

import (
	"sort"
	"time"
)

// CandidateNode is a schedulable node for an order, with its capability price
// and the score used to rank it.
type CandidateNode struct {
	NodeId      string  `json:"node_id"`
	PriceMicros int64   `json:"price_micros"`
	Score       float64 `json:"score"`
}

// maxScoreExecutions caps the executions used in the experience component of the
// candidate score so a very high-volume node can't dominate purely on count;
// beyond this the experience term is saturated and success rate/price decide.
const maxScoreExecutions = 50.0

// ScriptOffer is a Provider's public offer for a script version: its execution
// price, online/idle signal, provider group and execution track record. Clients
// browse offers before ordering.
//
// With concurrent execution a node is "busy" only when it has reached its total
// capacity (active leases >= sum of capability concurrency). AvailableSlots and
// TotalSlots expose how many concurrent tasks the node can currently accept for
// the selected script, letting the buyer gauge provider load at a glance.
type ScriptOffer struct {
	NodeId            string `json:"node_id"`
	ProviderGroupId   string `json:"provider_group_id,omitempty"`
	ProviderGroupName string `json:"provider_group_name,omitempty"`
	PriceMicros       int64  `json:"price_micros"`
	Online            bool   `json:"online"`
	// Busy is true when the node is online but has no available slots for the
	// selected script (either the node's total capacity or this script's per-node
	// concurrency limit is exhausted).
	Busy           bool `json:"busy"`
	// Concurrency is this script's per-node concurrency (from the capability).
	Concurrency    int  `json:"concurrency"`
	// AvailableSlots / TotalSlots describe the node's current capacity for this
	// script: how many more tasks it can accept right now, and its total limit.
	AvailableSlots int  `json:"available_slots"`
	TotalSlots     int  `json:"total_slots"`
	RemainingQuota    int    `json:"remaining_quota"`
	State             string `json:"state"`
	Executions        int64  `json:"executions"`
	Successes         int64  `json:"successes"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	Enabled           bool   `json:"enabled"`
	Owned             bool   `json:"owned"`
}

// ListOffersForScript returns all active, tested offers for a script version
// (online or not), cheapest first. With concurrent execution, "busy" means the
// node has no available slots for this script (either total node capacity or
// per-script limit is exhausted). AvailableSlots/TotalSlots show the node's
// current concurrency state for this script.
func ListOffersForScript(scriptId, version int, providerGroupId string, consumeMultiplier int64, viewerUserId int) ([]ScriptOffer, error) {
	if consumeMultiplier < 1 {
		consumeMultiplier = 1
	}
	cutoff := time.Now().Add(-NodePresenceTimeout).Unix()
	now := time.Now().Unix()
	type row struct {
		NodeId            string
		UserId            int
		ProviderGroupId   string
		ProviderGroupName string
		PriceMicros       int64
		Concurrency       int
		RemainingQuota    int
		LastSeenAt        int64
		State             string
		SuccessCount      int64
		FailureCount      int64
		TestExpiresAt     int64
		CategoryId        int
		BalanceOk         bool
		BalanceExpiresAt  int64
		Enabled           bool
	}
	var rows []row
	q := DB.Table("node_capabilities AS cap").
		Select(`cap.node_id, cap.price_micros, cap.concurrency, cap.remaining_quota,
			cap.test_expires_at, cap.category_id,
			n.user_id, n.last_seen_at, n.state, n.success_count, n.failure_count,
			n.provider_group_id, n.enabled,
			pg.name AS provider_group_name,
			s.balance_ok, s.expires_at AS balance_expires_at`).
		Joins("JOIN nodes n ON n.id = cap.node_id").
		Joins("LEFT JOIN provider_groups pg ON pg.id = n.provider_group_id").
		Joins("LEFT JOIN node_site_status s ON s.node_id = cap.node_id AND s.category_id = cap.category_id").
		Where("cap.script_id = ? AND cap.version = ?", scriptId, version).
		Where("cap.status = ?", CapabilityStatusActive).
		Where("n.enabled = ? OR n.user_id = ?", true, viewerUserId)
	if providerGroupId != "" {
		q = q.Where("n.provider_group_id = ?", providerGroupId)
	}
	if err := q.Order("cap.price_micros asc").Scan(&rows).Error; err != nil {
		return nil, err
	}

	// For each node compute active lease counts in one batch query.
	nodeIds := make([]string, 0, len(rows))
	for _, r := range rows {
		nodeIds = append(nodeIds, r.NodeId)
	}
	// nodeActiveLease: total active leases per node.
	type leaseStat struct {
		NodeId      string
		TotalActive int64
		ScriptActive int64
	}
	var totalLeaseStats []struct {
		NodeId string
		Count  int64
	}
	var scriptLeaseStats []struct {
		NodeId string
		Count  int64
	}
	if len(nodeIds) > 0 {
		DB.Table("leases").
			Select("node_id, COUNT(*) AS count").
			Where("node_id IN ? AND active = ?", nodeIds, true).
			Group("node_id").
			Scan(&totalLeaseStats)
		DB.Table("leases").
			Select("node_id, COUNT(*) AS count").
			Where("node_id IN ? AND script_id = ? AND version = ? AND active = ?", nodeIds, scriptId, version, true).
			Group("node_id").
			Scan(&scriptLeaseStats)
	}
	totalLeaseByNode := make(map[string]int64, len(totalLeaseStats))
	for _, s := range totalLeaseStats {
		totalLeaseByNode[s.NodeId] = s.Count
	}
	scriptLeaseByNode := make(map[string]int64, len(scriptLeaseStats))
	for _, s := range scriptLeaseStats {
		scriptLeaseByNode[s.NodeId] = s.Count
	}
	// Total node concurrency (sum of all active capabilities' concurrency) per node.
	var nodeTotalCap []struct {
		NodeId string
		Total  int64
	}
	if len(nodeIds) > 0 {
		DB.Table("node_capabilities").
			Select("node_id, COALESCE(SUM(concurrency), 1) AS total").
			Where("node_id IN ? AND status = ?", nodeIds, CapabilityStatusActive).
			Group("node_id").
			Scan(&nodeTotalCap)
	}
	nodeTotalByNode := make(map[string]int64, len(nodeTotalCap))
	for _, c := range nodeTotalCap {
		if c.Total < 1 {
			c.Total = 1
		}
		nodeTotalByNode[c.NodeId] = c.Total
	}

	offers := make([]ScriptOffer, 0, len(rows))
	for _, r := range rows {
		online := r.State != NodeStateOffline && r.LastSeenAt >= cutoff
		scriptConcurrency := r.Concurrency
		if scriptConcurrency < 1 {
			scriptConcurrency = 1
		}
		nodeTotalConcurrency := nodeTotalByNode[r.NodeId]
		if nodeTotalConcurrency < 1 {
			nodeTotalConcurrency = 1
		}
		totalActive := totalLeaseByNode[r.NodeId]
		scriptActive := scriptLeaseByNode[r.NodeId]

		// Node is at capacity when total active >= total node concurrency.
		nodeAtCap := totalActive >= nodeTotalConcurrency
		// This script's slot on this node is exhausted.
		scriptAtCap := scriptActive >= int64(scriptConcurrency)
		busy := online && (nodeAtCap || scriptAtCap)

		// Available slots for this specific script on this node.
		scriptAvailSlots := int64(scriptConcurrency) - scriptActive
		nodeAvailSlots := nodeTotalConcurrency - totalActive
		availSlots := scriptAvailSlots
		if nodeAvailSlots < availSlots {
			availSlots = nodeAvailSlots
		}
		if availSlots < 0 {
			availSlots = 0
		}

		testValid := r.TestExpiresAt > now
		balanceValid := r.CategoryId == 0 || (r.BalanceOk && r.BalanceExpiresAt > now)
		balanceEnough := int64(r.RemainingQuota) > consumeMultiplier
		owned := r.UserId == viewerUserId
		enabledOrOwned := r.Enabled || owned
		available := enabledOrOwned && online && !busy && balanceEnough && testValid && balanceValid
		reason := ""
		if !enabledOrOwned {
			reason = "NODE_DISABLED"
		} else if !online {
			reason = "NODE_OFFLINE"
		} else if busy {
			reason = "NODE_BUSY"
		} else if r.RemainingQuota <= 0 {
			reason = "QUOTA_EXHAUSTED"
		} else if !balanceEnough {
			reason = "INSUFFICIENT_NODE_BALANCE"
		} else if !testValid {
			reason = "CAPABILITY_TEST_EXPIRED"
		} else if !balanceValid {
			reason = "BALANCE_CHECK_EXPIRED"
		}
		offers = append(offers, ScriptOffer{
			NodeId:            r.NodeId,
			ProviderGroupId:   r.ProviderGroupId,
			ProviderGroupName: r.ProviderGroupName,
			PriceMicros:       r.PriceMicros,
			Online:            online,
			Busy:              busy,
			Concurrency:       scriptConcurrency,
			AvailableSlots:    int(availSlots),
			TotalSlots:        scriptConcurrency,
			RemainingQuota:    r.RemainingQuota,
			State:             r.State,
			Executions:        r.SuccessCount + r.FailureCount,
			Successes:         r.SuccessCount,
			Available:         available,
			Enabled:           r.Enabled,
			Owned:             owned,
			UnavailableReason: reason,
		})
	}
	return offers, nil
}

// GetCapabilityPrice returns a specific node's price for a script version, and
// whether that capability is currently active. Used to resolve the provider
// price a client selected.
func GetCapabilityPrice(nodeId string, scriptId, version int) (int64, bool, error) {
	var cap NodeCapability
	err := DB.Where("node_id = ? AND script_id = ? AND version = ?", nodeId, scriptId, version).First(&cap).Error
	if err != nil {
		return 0, false, err
	}
	return cap.PriceMicros, cap.Status == CapabilityStatusActive && cap.IsTestValid(), nil
}

// ScheduleCandidates returns eligible nodes for a script version whose price is
// within maxPriceMicros, ranked by score. With concurrent execution a node is
// eligible as long as it has available capacity: its active lease count is below
// both the total node concurrency (sum of all capability concurrency values) AND
// the per-script concurrency limit for this specific capability.
func ScheduleCandidates(scriptId, version int, maxPriceMicros int64, limit int, providerGroupId string, consumeMultiplier int64, clientUserId int, chosenNodeId string) ([]CandidateNode, error) {
	if consumeMultiplier < 1 {
		consumeMultiplier = 1
	}
	cutoff := time.Now().Add(-NodePresenceTimeout).Unix()

	type row struct {
		NodeId       string
		PriceMicros  int64
		SuccessCount int64
		FailureCount int64
	}
	var rows []row
	now := time.Now().Unix()
	// A node is schedulable when its active lease count is below BOTH its total
	// concurrency (node-level) and its per-script concurrency (script-level).
	// This replaces the old "n.state = IDLE" filter which only allowed one task.
	q := DB.Table("node_capabilities AS cap").
		Select("cap.node_id AS node_id, cap.price_micros AS price_micros, n.success_count AS success_count, n.failure_count AS failure_count").
		Joins("JOIN nodes n ON n.id = cap.node_id").
		Joins("LEFT JOIN node_site_status s ON s.node_id = cap.node_id AND s.category_id = cap.category_id").
		Where("cap.script_id = ? AND cap.version = ?", scriptId, version).
		Where("cap.status = ?", CapabilityStatusActive).
		Where("cap.test_expires_at > ?", now).
		Where("cap.remaining_quota > ?", consumeMultiplier).
		Where("cap.price_micros <= ?", maxPriceMicros).
		Where("n.last_seen_at >= ?", cutoff).
		Where("n.state != ?", NodeStateOffline).
		// Node-level capacity: total active leases < sum of all cap concurrency.
		Where(`(SELECT COUNT(*) FROM leases l WHERE l.node_id = cap.node_id AND l.active = ?) < `+
			`(SELECT COALESCE(SUM(nc2.concurrency), 1) FROM node_capabilities nc2 `+
			`WHERE nc2.node_id = cap.node_id AND nc2.status = ?)`,
			true, CapabilityStatusActive).
		// Per-script capacity: active leases for this script < this cap's concurrency.
		Where(`(SELECT COUNT(*) FROM leases l WHERE l.node_id = cap.node_id AND l.script_id = ? AND l.version = ? AND l.active = ?) < `+
			`COALESCE(cap.concurrency, 1)`,
			scriptId, version, true).
		// Node must have a valid balance check for the category (or no category).
		Where("cap.category_id = 0 OR (s.balance_ok = ? AND s.expires_at > ?)", true, now)
	if chosenNodeId != "" {
		q = q.Where("n.enabled = ? OR (n.id = ? AND n.user_id = ?)", true, chosenNodeId, clientUserId)
	} else {
		q = q.Where("n.enabled = ?", true)
	}
	if providerGroupId != "" {
		q = q.Where("n.provider_group_id = ?", providerGroupId)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}

	candidates := make([]CandidateNode, 0, len(rows))
	for _, r := range rows {
		n := Node{SuccessCount: r.SuccessCount, FailureCount: r.FailureCount}
		candidates = append(candidates, CandidateNode{
			NodeId:      r.NodeId,
			PriceMicros: r.PriceMicros,
			Score:       scoreCandidate(n.SuccessRate(), r.SuccessCount+r.FailureCount, r.PriceMicros, maxPriceMicros),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].PriceMicros != candidates[j].PriceMicros {
			return candidates[i].PriceMicros < candidates[j].PriceMicros
		}
		return candidates[i].NodeId < candidates[j].NodeId
	})
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

// scoreCandidate ranks an idle node by a weighted blend so "auto" picks the
// most-executed, highest-success idle provider (product requirement 3), with
// price as a minor tie-breaker:
//   - success rate (weight 0.55): reliability dominates.
//   - experience (weight 0.30): executions normalized to [0,1] against a cap, so
//     a proven high-volume node outranks an untried one at similar reliability.
//   - price (weight 0.15): cheaper breaks near-ties.
// All three are in [0,1]. A low-success/untried node still only wins when the
// better ones are busy (excluded from candidates upstream).
func scoreCandidate(successRate float64, executions, priceMicros, maxPriceMicros int64) float64 {
	priceScore := 0.0
	if maxPriceMicros > 0 {
		priceScore = 1.0 - float64(priceMicros)/float64(maxPriceMicros)
		if priceScore < 0 {
			priceScore = 0
		}
	}
	experienceScore := float64(executions) / maxScoreExecutions
	if experienceScore > 1 {
		experienceScore = 1
	}
	return 0.55*successRate + 0.30*experienceScore + 0.15*priceScore
}
