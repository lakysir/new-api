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
	ScriptId        int
	Version         int
	NodeId          string // optional: chosen provider offer
	ProviderGroupId string // optional: restrict auto-pick to a provider group
	RelayGB         float64
	StorageGBHours  float64
	// ConsumeMultiplier scales the execution cost (units of work). Defaults to 1
	// when zero; the pricing layer clamps values < 1 up to 1.
	ConsumeMultiplier int64
	// ClientId is the requesting user, so a provider can quote/select their own
	// disabled node (self-test). Zero means "no owner context" (public offers).
	ClientId int
}

// Quote is the itemized price returned to the client before ordering.
//
// In chosen-node mode Breakdown, BreakdownMin and BreakdownMax are identical.
// In auto mode the provider price spans the available offers (each node applies
// its own multiplier), so BreakdownMin/BreakdownMax bracket the range shown to
// the buyer. Breakdown carries the default reservation (the computed max total);
// the buyer may lower the cap, and dispatch then only picks nodes whose total
// stays within it. Settlement pays the actual node's cost and releases the rest.
type Quote struct {
	Breakdown    pricing.Breakdown
	BreakdownMin pricing.Breakdown
	BreakdownMax pricing.Breakdown
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

// providerPrice is the resolved provider bid range for a quote. In chosen-node
// mode Min == Max == that node's price. In auto mode Min/Max bracket the
// available offers and Reserve is the price used to size the reservation.
type providerPrice struct {
	Min       int64  // cheapest candidate provider bid (per unit, pre-consume)
	Max       int64  // priciest candidate provider bid — reserved against
	Reserve   int64  // the bid to reserve/size MaxCustomerMicros against (== Max)
	ChosenNode string // non-empty only when the client picked a specific node
}

// resolveProviderPrice determines the provider execution price range for a quote:
//   - if NodeId is set, use that node's capability price (client picked an offer);
//   - else span the offers for the version. The scheduler ranks by score (success
//     rate + experience + price), so it may pick a node that is NOT the cheapest;
//     the reservation therefore sizes against the MAX candidate price so the
//     chosen node is always fully covered. The unused remainder is released back
//     to the buyer at settlement.
func resolveProviderPrice(scriptId, version int, nodeId, providerGroupId string, consumeMultiplier int64, viewerUserId int) (providerPrice, error) {
	if nodeId != "" {
		price, ok, err := model.GetCapabilityPrice(nodeId, scriptId, version)
		if err != nil || !ok {
			return providerPrice{}, ErrNoOffer
		}
		return providerPrice{Min: price, Max: price, Reserve: price, ChosenNode: nodeId}, nil
	}
	offers, err := model.ListOffersForScript(scriptId, version, providerGroupId, consumeMultiplier, viewerUserId)
	if err != nil {
		return providerPrice{}, err
	}
	if len(offers) == 0 {
		return providerPrice{}, ErrNoOffer
	}
	// Span the candidate offers. Prefer available (idle) offers for the range so
	// the quote reflects nodes that can actually run now; if none are available,
	// fall back to the full offer set so the buyer still sees a price.
	var min, max int64
	found := false
	consider := func(price int64) {
		if !found {
			min, max, found = price, price, true
			return
		}
		if price < min {
			min = price
		}
		if price > max {
			max = price
		}
	}
	for _, o := range offers {
		if o.Available {
			consider(o.PriceMicros)
		}
	}
	if !found {
		for _, o := range offers {
			consider(o.PriceMicros)
		}
	}
	// Reserve against the priciest candidate so any scheduler pick is covered.
	return providerPrice{Min: min, Max: max, Reserve: max}, nil
}

// ErrNoOffer is returned when no active provider offer exists for a version.
var ErrNoOffer = errors.New("no provider offer available for this script version")

// NodeTotalMicros computes the TOTAL customer amount for running a script version
// on a specific node bid, using the version's pricing template and consume
// multiplier. Relay/storage reserves are passed through from the frozen values
// (they are usage-based, not bid-derived) so the result matches what settlement
// pays. Used by dispatch (skip candidates over the buyer's cap) and settlement
// (recompute the split at the actual executing node). Returns the full breakdown.
func NodeTotalMicros(scriptId, version int, nodeBidMicros, consumeMultiplier, relayMicros, storageMicros int64) (pricing.Breakdown, error) {
	tpl, err := resolveTemplate(scriptId, version)
	if err != nil {
		return pricing.Breakdown{}, err
	}
	// Compute the bid-derived lines with zero usage estimates, then overlay the
	// frozen relay/storage reserves so the total reflects the actual reservation.
	bd, err := pricing.Compute(nodeBidMicros, tpl.ToPricingTemplate(), pricing.Estimate{
		ConsumeMultiplier: normalizeMultiplier(consumeMultiplier),
	})
	if err != nil {
		return pricing.Breakdown{}, err
	}
	bd.RelayFeeMicros = relayMicros
	bd.StorageFeeMicros = storageMicros
	bd.MaxCustomerMicros = bd.ProviderMicros + bd.AuthorMicros + bd.PlatformFeeMicros +
		bd.RelayFeeMicros + bd.StorageFeeMicros + bd.RiskReserveMicros
	return bd, nil
}

// normalizeMultiplier floors the consume multiplier at 1 so a zero/negative
// value (e.g. an omitted field) never under-charges or disables the balance gate.
func normalizeMultiplier(m int64) int64 {
	if m < 1 {
		return 1
	}
	return m
}

// GetQuote computes a price breakdown using the real provider price (chosen
// offer or cheapest). The client never sets the provider's cut — only which
// offer / a max price.
func GetQuote(req QuoteRequest) (*Quote, error) {
	tpl, err := resolveTemplate(req.ScriptId, req.Version)
	if err != nil {
		return nil, err
	}
	pp, err := resolveProviderPrice(req.ScriptId, req.Version, req.NodeId, req.ProviderGroupId, normalizeMultiplier(req.ConsumeMultiplier), req.ClientId)
	if err != nil {
		return nil, err
	}
	est := pricing.Estimate{
		RelayGB: req.RelayGB, StorageGBHours: req.StorageGBHours,
		ConsumeMultiplier: normalizeMultiplier(req.ConsumeMultiplier),
	}
	pt := tpl.ToPricingTemplate()
	// Reserve breakdown uses the MAX candidate bid so the reservation covers any
	// node the scheduler picks. Min/Max bracket the range shown to the buyer.
	bdReserve, err := pricing.Compute(pp.Reserve, pt, est)
	if err != nil {
		return nil, err
	}
	bdMin, err := pricing.Compute(pp.Min, pt, est)
	if err != nil {
		return nil, err
	}
	bdMax, err := pricing.Compute(pp.Max, pt, est)
	if err != nil {
		return nil, err
	}
	return &Quote{
		Breakdown: bdReserve, BreakdownMin: bdMin, BreakdownMax: bdMax,
		TemplateId: tpl.Id, ChosenNodeId: pp.ChosenNode,
	}, nil
}

// CreateRequest is a client's order creation input. Only metadata and the input
// hash cross the control plane; parameters go over the E2EE data plane (T-004).
type CreateRequest struct {
	ClientId        int
	ScriptId        int
	Version         int
	NodeId          string // optional: chosen provider offer
	ProviderGroupId string // optional: restrict auto-pick to a provider group
	InputHash       string
	IdempotencyKey  string
	RelayGB         float64
	StorageGBHours  float64
	// ConsumeMultiplier scales the execution cost (units of work). Defaults to 1.
	ConsumeMultiplier int64
	// MaxAmountMicros is the buyer's ceiling on the TOTAL customer amount (provider
	// + author + platform + reserves). Zero means "use the computed max across
	// available offers". When set it must be >= the quote's minimum total, else no
	// node fits; the amount is reserved as-is and dispatch only picks nodes whose
	// total cost stays within it. Settlement pays the actual node's cost and
	// releases the remainder.
	MaxAmountMicros int64
}

// Create makes an idempotent order: it prices the bid, snapshots the breakdown
// and inserts the order atomically. Re-submitting the same idempotency key
// returns the existing order without reserving funds again (T-002).
func Create(req CreateRequest) (*model.Order, bool, error) {
	multiplier := normalizeMultiplier(req.ConsumeMultiplier)
	quote, err := GetQuote(QuoteRequest{
		ScriptId: req.ScriptId, Version: req.Version, NodeId: req.NodeId,
		ProviderGroupId: req.ProviderGroupId,
		RelayGB:         req.RelayGB, StorageGBHours: req.StorageGBHours,
		ConsumeMultiplier: multiplier,
		ClientId:          req.ClientId,
	})
	if err != nil {
		return nil, false, err
	}
	bd := quote.Breakdown

	// Reserve the buyer's chosen ceiling when supplied, else the computed max
	// across offers. Clamp to at least the minimum total so a too-low cap can
	// never reserve below one runnable node's cost (dispatch would then find no
	// candidate, which surfaces as a clear "no offer within budget").
	reserveMicros := bd.MaxCustomerMicros
	if req.MaxAmountMicros > 0 {
		reserveMicros = req.MaxAmountMicros
		if reserveMicros < quote.BreakdownMin.MaxCustomerMicros {
			reserveMicros = quote.BreakdownMin.MaxCustomerMicros
		}
	}

	o := &model.Order{
		Id:                model.NewOrderId(),
		ClientId:          req.ClientId,
		ScriptId:          req.ScriptId,
		Version:           req.Version,
		State:             model.OrderCreated,
		InputHash:         req.InputHash,
		IdempotencyKey:    req.IdempotencyKey,
		ChosenNodeId:      quote.ChosenNodeId,
		ProviderGroupId:   req.ProviderGroupId,
		MaxAmountMicros:   reserveMicros,
		ConsumeMultiplier: multiplier,
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
