package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/order"
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
	offers, err := model.ListOffersForScript(scriptId, version)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, offers)
}

type quoteRequest struct {
	ScriptId       int     `json:"script_id"`
	Version        int     `json:"version"`
	NodeId         string  `json:"node_id"` // optional: chosen provider offer
	RelayGB        float64 `json:"relay_gb"`
	StorageGBHours float64 `json:"storage_gb_hours"`
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
		RelayGB: req.RelayGB, StorageGBHours: req.StorageGBHours,
	})
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"breakdown": q.Breakdown, "chosen_node_id": q.ChosenNodeId})
}

type createOrderRequest struct {
	ScriptId       int     `json:"script_id"`
	Version        int     `json:"version"`
	NodeId         string  `json:"node_id"` // optional: chosen provider offer
	InputHash      string  `json:"input_hash"`
	RelayGB        float64 `json:"relay_gb"`
	StorageGBHours float64 `json:"storage_gb_hours"`
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
		ClientId:       c.GetInt("id"),
		ScriptId:       req.ScriptId,
		Version:        req.Version,
		NodeId:         req.NodeId,
		InputHash:      req.InputHash,
		IdempotencyKey: idempotencyKey,
		RelayGB:        req.RelayGB,
		StorageGBHours: req.StorageGBHours,
	})
	if err != nil {
		if errors.Is(err, order.ErrScriptNotExecutable) {
			common.ApiErrorMsg(c, "script version is not executable")
			return
		}
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"order": o, "created": created})
}

// GetOrder returns an order's current state (owner-scoped).
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
	common.ApiSuccess(c, updated)
}
