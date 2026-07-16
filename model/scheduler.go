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
// browse offers before ordering. A node that is online but already running a
// task (state BUSY) is reported not-available: a provider executes at most one
// script at a time (PRD N-006), even when it lists several capabilities.
type ScriptOffer struct {
	NodeId            string `json:"node_id"`
	ProviderGroupId   string `json:"provider_group_id,omitempty"`
	ProviderGroupName string `json:"provider_group_name,omitempty"`
	PriceMicros       int64  `json:"price_micros"`
	Online            bool   `json:"online"`
	// Busy is true when the node is online but currently executing a task and so
	// cannot accept a new one.
	Busy              bool   `json:"busy"`
	RemainingQuota    int    `json:"remaining_quota"`
	State             string `json:"state"`
	// Executions / Successes are this node's lifetime task outcomes (success +
	// failure counters), used to show a success rate in the same format as the
	// provider console.
	Executions        int64  `json:"executions"`
	Successes         int64  `json:"successes"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	// Enabled is the provider's scheduling switch for the node. Owned is true
	// when the offer's node belongs to the viewer. A disabled node is only listed
	// to its owner (so they can test it); for everyone else disabled nodes are
	// filtered out entirely.
	Enabled bool `json:"enabled"`
	Owned   bool `json:"owned"`
}

// ListOffersForScript returns all active, tested offers for a script version
// (online or not), cheapest first — the client-facing "how much does one
// execution cost" catalog (architecture §13.2). When providerGroupId is
// non-empty the result is restricted to nodes in that group so a client can
// filter by provider.
// consumeMultiplier is the buyer's units-of-work coefficient (min 1): a node's
// remaining balance must be strictly greater than it for the offer to be
// available, since one execution consumes that many units on the target site.
// viewerUserId is the requesting user: their own disabled nodes are listed (and
// selectable, so they can test their nodes end-to-end), while disabled nodes
// owned by anyone else are filtered out.
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
	// Return all listed capabilities so clients can distinguish "no offer" from
	// an offer that is temporarily offline, out of quota or needs re-validation.
	// Disabled nodes are hidden from everyone except their owner.
	q := DB.Table("node_capabilities AS cap").
		Select(`cap.node_id, cap.price_micros, cap.remaining_quota, cap.test_expires_at, cap.category_id,
			n.user_id, n.last_seen_at, n.state, n.success_count, n.failure_count, n.provider_group_id, n.enabled,
			pg.name AS provider_group_name, s.balance_ok, s.expires_at AS balance_expires_at`).
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
	offers := make([]ScriptOffer, 0, len(rows))
	for _, r := range rows {
		online := r.State != NodeStateOffline && r.LastSeenAt >= cutoff
		busy := online && r.State == NodeStateBusy
		testValid := r.TestExpiresAt > now
		balanceValid := r.CategoryId == 0 || (r.BalanceOk && r.BalanceExpiresAt > now)
		// The node must hold enough target-site balance to consume this run's
		// units of work; > (not >=) so a node with exactly the coefficient is not
		// drained to zero mid-execution.
		balanceEnough := int64(r.RemainingQuota) > consumeMultiplier
		owned := r.UserId == viewerUserId
		// A disabled node is normally not a candidate; the owner may still select
		// their own disabled node to test it end-to-end (others' disabled nodes are
		// already filtered out by the query), so the enabled gate is waived when
		// the node is the viewer's. A busy node is online but cannot take a new
		// task (single-task-per-node).
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
// within maxPriceMicros, ranked by score (architecture §10). Candidate filter
// mirrors §10.1: online + IDLE + active capability + valid test + quota + price.
// Scoring in the MVP is a simplified static score; quality/latency signals are
// added in later phases.
// providerGroupId (optional) restricts candidates to a single provider group so
// the client can auto-pick within a chosen group (offer filtering); empty means
// all groups.
// consumeMultiplier (min 1) is the run's units of work: a node's remaining
// balance must be strictly greater than it to be eligible (mirrors
// ListOffersForScript's INSUFFICIENT_NODE_BALANCE gate).
// clientUserId + chosenNodeId let a provider test their own node: when the
// client explicitly chose a node they own, that specific node is eligible even
// if disabled. Auto-pick (chosenNodeId == "") still requires n.enabled = true,
// so a disabled node is never auto-selected for someone else.
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
	// Only IDLE (idle, not busy) nodes are candidates; busy nodes are excluded
	// by n.state = IDLE. Combined with success-rate ordering below, a
	// low-success node is only picked when all higher-success nodes are busy
	// (hence absent from this set) — exactly the desired fallback behavior.
	now := time.Now().Unix()
	q := DB.Table("node_capabilities AS cap").
		Select("cap.node_id AS node_id, cap.price_micros AS price_micros, n.success_count AS success_count, n.failure_count AS failure_count").
		Joins("JOIN nodes n ON n.id = cap.node_id").
		Joins("LEFT JOIN node_site_status s ON s.node_id = cap.node_id AND s.category_id = cap.category_id").
		Where("cap.script_id = ? AND cap.version = ?", scriptId, version).
		Where("cap.status = ?", CapabilityStatusActive).
		Where("cap.test_expires_at > ?", now).
		Where("cap.remaining_quota > ?", consumeMultiplier).
		Where("cap.price_micros <= ?", maxPriceMicros).
		Where("n.state = ?", NodeStateIdle).
		Where("n.last_seen_at >= ?", cutoff).
		// Node must have a valid balance check for the category (or no category).
		Where("cap.category_id = 0 OR (s.balance_ok = ? AND s.expires_at > ?)", true, now)
	// Enabled nodes are always eligible; a disabled node is eligible only when the
	// client explicitly chose that exact node and owns it (self-test path).
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
	// Higher score first; tie-break by lower price then node id for determinism.
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
