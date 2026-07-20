package controller

import (
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/dispatch"
	"github.com/QuantumNous/new-api/service/nodehub"
	"github.com/QuantumNous/new-api/service/order"
	"github.com/QuantumNous/new-api/service/settlement"
	"github.com/gin-gonic/gin"
)

// ListScriptOffers returns the provider offers (price + online + quota) for a
// script version — the client-facing "how much does one execution cost" list.
func ListScriptOffers(c *gin.Context) {
	scriptId, err := strconv.Atoi(c.Param("id"))
	if err != nil || scriptId <= 0 {
		common.ApiErrorMsg(c, "invalid script id")
		return
	}
	version, err := strconv.Atoi(c.Query("version"))
	if err != nil || version <= 0 {
		common.ApiErrorMsg(c, "version query param is required")
		return
	}
	// Optional provider_group_id filter: restrict offers to a single provider.
	providerGroupId := c.Query("provider_group_id")
	// Optional consume_multiplier: a node's remaining balance must exceed it for
	// the offer to be available, mirroring the auto-select balance gate.
	consumeMultiplier, _ := strconv.ParseInt(c.Query("consume_multiplier"), 10, 64)
	// The viewer sees their own disabled nodes (to test them); others' disabled
	// nodes are hidden.
	offers, err := model.ListOffersForScript(scriptId, version, providerGroupId, normalizeConsumeMultiplier(consumeMultiplier), c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, offers)
}

// normalizeConsumeMultiplier floors the buyer-supplied units-of-work coefficient
// at 1 so an omitted or invalid value never under-charges or disables the
// node-balance gate.
func normalizeConsumeMultiplier(m int64) int64 {
	if m < 1 {
		return 1
	}
	return m
}

type quoteRequest struct {
	ScriptId          int     `json:"script_id"`
	Version           int     `json:"version"`
	NodeId            string  `json:"node_id"`           // optional: chosen provider offer
	ProviderGroupId   string  `json:"provider_group_id"` // optional: restrict auto-pick to a group
	RelayGB           float64 `json:"relay_gb"`
	StorageGBHours    float64 `json:"storage_gb_hours"`
	ConsumeMultiplier int64   `json:"consume_multiplier"` // units of work; min 1
}

// QuoteOrder returns an itemized price breakdown for a script version using the
// real provider price (chosen offer or cheapest). The client never sets the
// provider's cut.
func QuoteOrder(c *gin.Context) {
	var req quoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	q, err := order.GetQuote(order.QuoteRequest{
		ScriptId: req.ScriptId, Version: req.Version, NodeId: req.NodeId,
		ProviderGroupId: req.ProviderGroupId,
		RelayGB:         req.RelayGB, StorageGBHours: req.StorageGBHours,
		ConsumeMultiplier: normalizeConsumeMultiplier(req.ConsumeMultiplier),
		ClientId:          c.GetInt("id"),
	})
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"breakdown": q.Breakdown, "chosen_node_id": q.ChosenNodeId})
}

type createOrderRequest struct {
	ScriptId          int     `json:"script_id"`
	Version           int     `json:"version"`
	NodeId            string  `json:"node_id"`           // optional: chosen provider offer
	ProviderGroupId   string  `json:"provider_group_id"` // optional: restrict auto-pick to a group
	InputHash         string  `json:"input_hash"`
	RelayGB           float64 `json:"relay_gb"`
	StorageGBHours    float64 `json:"storage_gb_hours"`
	ConsumeMultiplier int64   `json:"consume_multiplier"` // units of work; min 1
}

// CreateOrder creates an idempotent order. The Idempotency-Key header dedupes
// retries so a repeated request never reserves funds twice (T-002). Only
// metadata + input_hash are accepted; parameters travel the E2EE data plane.
func CreateOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		common.ApiErrorMsg(c, "Idempotency-Key header is required")
		return
	}
	o, created, err := order.Create(order.CreateRequest{
		ClientId:        c.GetInt("id"),
		ScriptId:        req.ScriptId,
		Version:         req.Version,
		NodeId:          req.NodeId,
		ProviderGroupId: req.ProviderGroupId,
		InputHash:         req.InputHash,
		IdempotencyKey:    idempotencyKey,
		RelayGB:           req.RelayGB,
		StorageGBHours:    req.StorageGBHours,
		ConsumeMultiplier: normalizeConsumeMultiplier(req.ConsumeMultiplier),
	})
	if err != nil {
		if errors.Is(err, order.ErrScriptNotExecutable) {
			common.ApiErrorMsg(c, "script version is not executable")
			return
		}
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if o.State == model.OrderCreated {
		o, err = settlement.ReserveFunds(o.Id)
		if err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		result, dispatchErr := dispatch.Dispatch(o.Id, 1)
		if dispatchErr != nil && !errors.Is(dispatchErr, dispatch.ErrNoCandidates) {
			common.ApiErrorMsg(c, dispatchErr.Error())
			return
		}
		if errors.Is(dispatchErr, dispatch.ErrNoCandidates) {
			// No idle provider matched (all busy or offline). The order is left in
			// MATCHING with funds reserved; nothing re-dispatches it, so it would
			// only sit frozen until the stale-order sweep refunds it ~10min later,
			// while the client waits out a confusing relay handshake timeout. Cancel
			// and refund now, and tell the caller plainly.
			if _, terr := model.ApplyTransition(o.Id, model.OrderCancelled, nil); terr == nil {
				_, _ = settlement.Refund(o.Id)
			}
			common.ApiErrorMsg(c, "no idle provider available right now (all providers are busy or offline); reserved funds were refunded")
			return
		}
		if result != nil {
			// Fast path: deliver this order's offer immediately. The transactional
			// Outbox remains the retry source if the socket disappears mid-send.
			if publishErr := dispatch.PublishEvent(nodehub.Default, result.EventId); publishErr != nil {
				if ta, _ := model.GetTaskAttempt(o.Id, 1); ta != nil {
					_ = model.ReleaseLease(ta.LeaseId, "offer_delivery_failed")
				}
				if current, _ := model.GetOrder(o.Id); current != nil && current.State == model.OrderOffered {
					_, _ = model.ApplyTransition(o.Id, model.OrderCancelled, nil)
					_, _ = settlement.Refund(o.Id)
				}
				common.ApiErrorMsg(c, "provider control channel is unavailable; reserved funds were refunded")
				return
			}
			// The control-channel response is asynchronous. Wait briefly for the
			// Provider to accept or reject so the client never starts Relay from a
			// transient OFFERED snapshot that has already been refunded.
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				current, getErr := model.GetOrder(o.Id)
				if getErr == nil && current.State != model.OrderOffered {
					o = current
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
		o, _ = model.GetOrder(o.Id)
	}
	common.ApiSuccess(c, gin.H{"order": o, "created": created})
}

// GetOrder returns an order's current state (owner-scoped). When the order is
// in a failure state, the latest attempt's error_code is surfaced as last_error
// so the client can show the real reason (e.g. ORIGIN_NOT_ALLOWED) instead of a
// generic relay timeout.
func GetOrder(c *gin.Context) {
	id := c.Param("id")
	o, err := model.GetOrder(id)
	if err != nil {
		common.ApiErrorMsg(c, "order not found")
		return
	}
	if o.ClientId != c.GetInt("id") {
		common.ApiErrorMsg(c, "order not found")
		return
	}
	switch o.State {
	case model.OrderFailed, model.OrderRefunded, model.OrderTimedOut, model.OrderCancelled:
		if ta, _ := model.GetTaskAttempt(id, 1); ta != nil {
			o.LastError = ta.ErrorCode
		}
	}
	common.ApiSuccess(c, o)
}

// CancelOrder requests cancellation. Only pre-execution states can be cancelled
// by the client; the state machine rejects illegal transitions.
func CancelOrder(c *gin.Context) {
	id := c.Param("id")
	o, err := model.GetOrder(id)
	if err != nil || o.ClientId != c.GetInt("id") {
		common.ApiErrorMsg(c, "order not found")
		return
	}
	updated, err := model.ApplyTransition(id, model.OrderCancelled, nil)
	if err != nil {
		if errors.Is(err, model.ErrIllegalTransition) {
			common.ApiErrorMsg(c, "order cannot be cancelled in its current state")
			return
		}
		common.ApiError(c, err)
		return
	}
	if ta, _ := model.GetTaskAttempt(id, 1); ta != nil {
		_ = model.ReleaseLease(ta.LeaseId, "cancelled")
	}
	updated, err = settlement.Refund(updated.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, updated)
}
