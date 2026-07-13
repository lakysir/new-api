// Package pricing computes order price breakdowns from a provider bid and a
// pricing template. All money is integer micro-USD; all rates are ppm (parts
// per million). It is free of HTTP/GORM concerns so the money math can be unit
// tested exhaustively (architecture §14.3).
package pricing

import "errors"

// PPMDenominator is the parts-per-million base: 1_000_000 ppm = 100%.
const PPMDenominator = 1_000_000

// ProviderPriceMode enumerates how the provider bid is interpreted. The MVP
// settles per-task; other modes are recorded for forward compatibility.
const (
	ModePerTask     = "per_task"
	ModePerDuration = "per_duration"
	ModePerOutput   = "per_output"
)

// FailurePolicy governs settlement when execution fails.
const (
	FailureFullRefund = "full_refund"
	FailurePartial    = "partial"
)

var (
	// ErrNegativeBid is returned for a negative provider bid.
	ErrNegativeBid = errors.New("provider bid must be non-negative")
	// ErrBadRate is returned when a ppm rate is out of [0, PPMDenominator].
	ErrBadRate = errors.New("rate ppm out of range")
)

// Template is the immutable pricing rule bound to a script version. Mirrors the
// pricing_templates columns; kept as a plain struct so pricing is pure.
type Template struct {
	Currency                    string
	ProviderPriceMode           string
	PlatformFeeRatePPM          int64
	PlatformFeeMinMicros        int64
	AuthorShareRatePPM          int64
	RelayPricePerGBMicros       int64
	StoragePricePerGBHourMicros int64
	RiskReserveRatePPM          int64
	FailurePolicy               string
	RuleVersion                 string
}

// Estimate carries optional usage estimates used for reserved fees at quote time.
type Estimate struct {
	RelayGB        float64 // estimated relay traffic
	StorageGBHours float64 // estimated temporary storage
}

// Breakdown is the itemized result. All fields are micro-USD. MaxCustomerMicros
// is what gets reserved from the client's balance; the sum of the component
// lines equals it exactly (architecture §14.4 invariant).
type Breakdown struct {
	Currency          string
	ProviderMicros    int64
	AuthorMicros      int64
	PlatformFeeMicros int64
	RelayFeeMicros    int64
	StorageFeeMicros  int64
	RiskReserveMicros int64
	MaxCustomerMicros int64
	RuleVersion       string
}

// applyRate returns floor(base * ratePPM / 1_000_000) using 128-bit-safe int64
// math (base and rate are bounded well below overflow for realistic amounts).
func applyRate(base, ratePPM int64) int64 {
	return base * ratePPM / PPMDenominator
}

// ceilMicros converts a float GB/hour estimate * per-unit price to micros,
// rounding up so the platform never under-reserves fractional usage.
func ceilMicros(units float64, perUnitMicros int64) int64 {
	if units <= 0 || perUnitMicros <= 0 {
		return 0
	}
	total := units * float64(perUnitMicros)
	i := int64(total)
	if float64(i) < total {
		i++ // round up
	}
	return i
}

// validate checks a template's rate bounds.
func (t Template) validate() error {
	for _, r := range []int64{t.PlatformFeeRatePPM, t.AuthorShareRatePPM, t.RiskReserveRatePPM} {
		if r < 0 || r > PPMDenominator {
			return ErrBadRate
		}
	}
	return nil
}

// Compute derives the full price breakdown from a provider bid, template and
// usage estimate. The provider always receives its bid; author share, platform
// fee and risk reserve are computed off the provider bid as the base. Network
// and storage fees are reserved from estimates. The max customer amount is the
// exact sum of all lines, so ledger entries balance by construction.
func Compute(bidMicros int64, t Template, est Estimate) (Breakdown, error) {
	if bidMicros < 0 {
		return Breakdown{}, ErrNegativeBid
	}
	if err := t.validate(); err != nil {
		return Breakdown{}, err
	}

	provider := bidMicros
	author := applyRate(bidMicros, t.AuthorShareRatePPM)

	platformFee := applyRate(bidMicros, t.PlatformFeeRatePPM)
	if platformFee < t.PlatformFeeMinMicros {
		platformFee = t.PlatformFeeMinMicros
	}

	risk := applyRate(bidMicros, t.RiskReserveRatePPM)
	relay := ceilMicros(est.RelayGB, t.RelayPricePerGBMicros)
	storage := ceilMicros(est.StorageGBHours, t.StoragePricePerGBHourMicros)

	max := provider + author + platformFee + relay + storage + risk

	return Breakdown{
		Currency:          t.Currency,
		ProviderMicros:    provider,
		AuthorMicros:      author,
		PlatformFeeMicros: platformFee,
		RelayFeeMicros:    relay,
		StorageFeeMicros:  storage,
		RiskReserveMicros: risk,
		MaxCustomerMicros: max,
		RuleVersion:       t.RuleVersion,
	}, nil
}

// SumComponents returns the sum of all component lines, used to assert the
// max-customer invariant in tests and at settlement.
func (b Breakdown) SumComponents() int64 {
	return b.ProviderMicros + b.AuthorMicros + b.PlatformFeeMicros +
		b.RelayFeeMicros + b.StorageFeeMicros + b.RiskReserveMicros
}
