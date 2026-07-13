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

// QuoteRequest asks for a price breakdown for a provider bid on a script version.
type QuoteRequest struct {
	ScriptId       int
	Version        int
	BidMicros      int64
	RelayGB        float64
	StorageGBHours float64
}

// Quote is the itemized price returned to the client before ordering.
type Quote struct {
	Breakdown  pricing.Breakdown
	TemplateId int
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

// GetQuote computes a price breakdown for a bid without creating an order.
func GetQuote(req QuoteRequest) (*Quote, error) {
	tpl, err := resolveTemplate(req.ScriptId, req.Version)
	if err != nil {
		return nil, err
	}
	bd, err := pricing.Compute(req.BidMicros, tpl.ToPricingTemplate(), pricing.Estimate{
		RelayGB: req.RelayGB, StorageGBHours: req.StorageGBHours,
	})
	if err != nil {
		return nil, err
	}
	return &Quote{Breakdown: bd, TemplateId: tpl.Id}, nil
}

// CreateRequest is a client's order creation input. Only metadata and the input
// hash cross the control plane; parameters go over the E2EE data plane (T-004).
type CreateRequest struct {
	ClientId       int
	ScriptId       int
	Version        int
	BidMicros      int64
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
		ScriptId: req.ScriptId, Version: req.Version, BidMicros: req.BidMicros,
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
