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

// ScriptOffer is a Provider's public offer for a script version: its execution
// price and online/quality signal. Clients browse offers before ordering.
type ScriptOffer struct {
	NodeId         string `json:"node_id"`
	PriceMicros    int64  `json:"price_micros"`
	Online         bool   `json:"online"`
	RemainingQuota int    `json:"remaining_quota"`
}

// ListOffersForScript returns all active, tested offers for a script version
// (online or not), cheapest first — the client-facing "how much does one
// execution cost" catalog (architecture §13.2).
func ListOffersForScript(scriptId, version int) ([]ScriptOffer, error) {
	cutoff := time.Now().Add(-NodePresenceTimeout).Unix()
	type row struct {
		NodeId         string
		PriceMicros    int64
		RemainingQuota int
		LastSeenAt     int64
		State          string
	}
	var rows []row
	// A capability only counts if the node also holds a valid balance check for
	// the capability's category (or the script has no category). LEFT JOIN so
	// category-less scripts still appear.
	now := time.Now().Unix()
	err := DB.Table("node_capabilities AS cap").
		Select("cap.node_id, cap.price_micros, cap.remaining_quota, n.last_seen_at, n.state").
		Joins("JOIN nodes n ON n.id = cap.node_id").
		Joins("LEFT JOIN node_site_status s ON s.node_id = cap.node_id AND s.category_id = cap.category_id").
		Where("cap.script_id = ? AND cap.version = ?", scriptId, version).
		Where("cap.status = ?", CapabilityStatusActive).
		Where("cap.test_expires_at > ?", now).
		Where("cap.category_id = 0 OR (s.balance_ok = ? AND s.expires_at > ?)", true, now).
		Order("cap.price_micros asc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	offers := make([]ScriptOffer, 0, len(rows))
	for _, r := range rows {
		offers = append(offers, ScriptOffer{
			NodeId:         r.NodeId,
			PriceMicros:    r.PriceMicros,
			Online:         r.State != NodeStateOffline && r.LastSeenAt >= cutoff,
			RemainingQuota: r.RemainingQuota,
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
func ScheduleCandidates(scriptId, version int, maxPriceMicros int64, limit int) ([]CandidateNode, error) {
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
	err := DB.Table("node_capabilities AS cap").
		Select("cap.node_id AS node_id, cap.price_micros AS price_micros, n.success_count AS success_count, n.failure_count AS failure_count").
		Joins("JOIN nodes n ON n.id = cap.node_id").
		Joins("LEFT JOIN node_site_status s ON s.node_id = cap.node_id AND s.category_id = cap.category_id").
		Where("cap.script_id = ? AND cap.version = ?", scriptId, version).
		Where("cap.status = ?", CapabilityStatusActive).
		Where("cap.test_expires_at > ?", now).
		Where("cap.remaining_quota > 0").
		Where("cap.price_micros <= ?", maxPriceMicros).
		Where("n.state = ?", NodeStateIdle).
		Where("n.last_seen_at >= ?", cutoff).
		// Node must have a valid balance check for the category (or no category).
		Where("cap.category_id = 0 OR (s.balance_ok = ? AND s.expires_at > ?)", true, now).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	candidates := make([]CandidateNode, 0, len(rows))
	for _, r := range rows {
		n := Node{SuccessCount: r.SuccessCount, FailureCount: r.FailureCount}
		candidates = append(candidates, CandidateNode{
			NodeId:      r.NodeId,
			PriceMicros: r.PriceMicros,
			Score:       scoreCandidate(n.SuccessRate(), r.PriceMicros, maxPriceMicros),
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

// scoreCandidate is the MVP price-only component of the §10.2 score, normalized
// to [0,1] where a lower price scores higher. Quality/stability/latency signals
// are layered in once receipts and metrics exist.
// scoreCandidate ranks a node by success rate (dominant) with price as a minor
// tie-breaker. Client prefers high-success nodes; a low-success node only wins
// when the higher ones are busy (excluded from candidates upstream).
func scoreCandidate(successRate float64, priceMicros, maxPriceMicros int64) float64 {
	priceScore := 0.0
	if maxPriceMicros > 0 {
		priceScore = 1.0 - float64(priceMicros)/float64(maxPriceMicros)
		if priceScore < 0 {
			priceScore = 0
		}
	}
	// Success rate weight 0.85 dominates; price weight 0.15 breaks near-ties.
	return 0.85*successRate + 0.15*priceScore
}
