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

// ScheduleCandidates returns eligible nodes for a script version whose price is
// within maxPriceMicros, ranked by score (architecture §10). Candidate filter
// mirrors §10.1: online + IDLE + active capability + valid test + quota + price.
// Scoring in the MVP is a simplified static score; quality/latency signals are
// added in later phases.
func ScheduleCandidates(scriptId, version int, maxPriceMicros int64, limit int) ([]CandidateNode, error) {
	cutoff := time.Now().Add(-NodePresenceTimeout).Unix()

	type row struct {
		NodeId      string
		PriceMicros int64
	}
	var rows []row
	err := DB.Table("node_capabilities AS cap").
		Select("cap.node_id AS node_id, cap.price_micros AS price_micros").
		Joins("JOIN nodes n ON n.id = cap.node_id").
		Where("cap.script_id = ? AND cap.version = ?", scriptId, version).
		Where("cap.status = ?", CapabilityStatusActive).
		Where("cap.test_expires_at > ?", time.Now().Unix()).
		Where("cap.remaining_quota > 0").
		Where("cap.price_micros <= ?", maxPriceMicros).
		Where("n.state = ?", NodeStateIdle).
		Where("n.last_seen_at >= ?", cutoff).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	candidates := make([]CandidateNode, 0, len(rows))
	for _, r := range rows {
		candidates = append(candidates, CandidateNode{
			NodeId:      r.NodeId,
			PriceMicros: r.PriceMicros,
			Score:       scoreCandidate(r.PriceMicros, maxPriceMicros),
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
func scoreCandidate(priceMicros, maxPriceMicros int64) float64 {
	if maxPriceMicros <= 0 {
		return 0
	}
	priceScore := 1.0 - float64(priceMicros)/float64(maxPriceMicros)
	if priceScore < 0 {
		priceScore = 0
	}
	// price weight 0.15 in the full model; other components default to a neutral
	// baseline until their signals are available.
	return 0.85*0.5 + 0.15*priceScore
}
