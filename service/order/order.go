// Package order wires pricing, script versions and orders into the create/quote
// use cases. Domain state transitions live in the model layer (ApplyTransition);
// this package composes them so controllers stay thin (architecture §22.4).
package order

import (
	"errors"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/pricing"
)

// ErrScriptNotExecutable is returned when the target version is missing/revoked.
var ErrScriptNotExecutable = errors.New("script version is not executable")

// QuoteRequest asks for a price breakdown for a script version. The client
// optionally picks a specific provider offer via NodeId; the provider's price
// comes from its capability, never from the client.
type QuoteRequest struct {
	ScriptId       int
	Version        int
	NodeId         string // optional: chosen provider offer
	RelayGB        float64
	StorageGBHours float64
}

// Quote is the itemized price returned to the client before ordering.
type Quote struct {
	Breakdown    pricing.Breakdown
	TemplateId   int
	ChosenNodeId string
}

// resolveTemplate loads the pricing template bound to a script version. When the
// version has no template bound, a zero-fee default is used so the MVP can still
// quote (architecture §14.3 binding).
func resolveTemplate(scriptId, version int) (*model.PricingTemplate, error) {
	sv, err := model.GetExecutableScriptVersion(scriptId, version)
	if err != nil {
		return nil, ErrScriptNotExecutable
	}
	if sv.PricingTemplateId > 0 {
		tpl, err := model.GetPricingTemplate(sv.PricingTemplateId)
		if err != nil {
			return nil, err
		}
		return tpl, nil
	}
	// No template bound: permissive zero-fee default.
	return &model.PricingTemplate{
		Currency:          "USD",
		ProviderPriceMode: pricing.ModePerTask,
		FailurePolicy:     pricing.FailureFullRefund,
		RuleVersion:       "default-v1",
	}, nil
}

// resolveProviderPrice determines the provider execution price for a quote:
// - if NodeId is set, use that node's capability price (client picked an offer);
// - else use the cheapest online offer for the script version.
// Returns the price and the chosen node id (empty if none available).
func resolveProviderPrice(scriptId, version int, nodeId string) (int64, string, error) {
	if nodeId != "" {
		price, ok, err := model.GetCapabilityPrice(nodeId, scriptId, version)
		if err != nil {
			return 0, "", ErrNoOffer
		}
		if !ok {
			return 0, "", ErrNoOffer
		}
		return price, nodeId, nil
	}
	offers, err := model.ListOffersForScript(scriptId, version)
	if err != nil {
		return 0, "", err
	}
	// Cheapest first; prefer an online offer.
	for _, o := range offers {
		if o.Online && o.RemainingQuota > 0 {
			return o.PriceMicros, o.NodeId, nil
		}
	}
	if len(offers) == 0 {
		return 0, "", ErrNoOffer
	}
	// No online offer: quote the cheapest so the client sees a price, but the
	// chosen node stays empty and the scheduler will match when one comes online.
	return offers[0].PriceMicros, "", nil
}

// ErrNoOffer is returned when no active provider offer exists for a version.
var ErrNoOffer = errors.New("no provider offer available for this script version")

// GetQuote computes a price breakdown using the real provider price (chosen
// offer or cheapest). The client never sets the provider's cut — only which
// offer / a max price.
func GetQuote(req QuoteRequest) (*Quote, error) {
	tpl, err := resolveTemplate(req.ScriptId, req.Version)
	if err != nil {
		return nil, err
	}
	providerMicros, chosenNode, err := resolveProviderPrice(req.ScriptId, req.Version, req.NodeId)
	if err != nil {
		return nil, err
	}
	bd, err := pricing.Compute(providerMicros, tpl.ToPricingTemplate(), pricing.Estimate{
		RelayGB: req.RelayGB, StorageGBHours: req.StorageGBHours,
	})
	if err != nil {
		return nil, err
	}
	return &Quote{Breakdown: bd, TemplateId: tpl.Id, ChosenNodeId: chosenNode}, nil
}

// CreateRequest is a client's order creation input. Only metadata and the input
// hash cross the control plane; parameters go over the E2EE data plane (T-004).
type CreateRequest struct {
	ClientId       int
	ScriptId       int
	Version        int
	NodeId         string // optional: chosen provider offer
	InputHash      string
	IdempotencyKey string
	RelayGB        float64
	StorageGBHours float64
}

// Create makes an idempotent order: it prices the bid, snapshots the breakdown
// and inserts the order atomically. Re-submitting the same idempotency key
// returns the existing order without reserving funds again (T-002).
func Create(req CreateRequest) (*model.Order, bool, error) {
	quote, err := GetQuote(QuoteRequest{
		ScriptId: req.ScriptId, Version: req.Version, NodeId: req.NodeId,
		RelayGB: req.RelayGB, StorageGBHours: req.StorageGBHours,
	})
	if err != nil {
		return nil, false, err
	}
	bd := quote.Breakdown

	o := &model.Order{
		Id:              model.NewOrderId(),
		ClientId:        req.ClientId,
		ScriptId:        req.ScriptId,
		Version:         req.Version,
		State:           model.OrderCreated,
		InputHash:       req.InputHash,
		IdempotencyKey:  req.IdempotencyKey,
		ChosenNodeId:    quote.ChosenNodeId,
		MaxAmountMicros: bd.MaxCustomerMicros,
	}
	snap := &model.OrderPriceSnapshot{
		Currency:                 bd.Currency,
		ProviderAmountMicros:     bd.ProviderMicros,
		AuthorAmountMicros:       bd.AuthorMicros,
		PlatformFeeMicros:        bd.PlatformFeeMicros,
		RelayFeeReservedMicros:   bd.RelayFeeMicros,
		StorageFeeReservedMicros: bd.StorageFeeMicros,
		RiskReserveMicros:        bd.RiskReserveMicros,
		MaxCustomerAmountMicros:  bd.MaxCustomerMicros,
		PricingRuleVersion:       bd.RuleVersion,
	}
	return model.CreateOrderWithSnapshot(o, snap)
}
