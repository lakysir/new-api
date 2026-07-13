package model

import (
	"errors"

	"github.com/QuantumNous/new-api/service/pricing"
)

// PricingTemplate is the immutable pricing rule bound to a script version. A
// rate change publishes a new template version rather than mutating in place
// (architecture §12 constraint).
type PricingTemplate struct {
	Id                          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Currency                    string `json:"currency" gorm:"type:varchar(8);default:USD"`
	ProviderPriceMode           string `json:"provider_price_mode" gorm:"type:varchar(16);default:per_task"`
	PlatformFeeRatePPM          int64  `json:"platform_fee_rate_ppm" gorm:"default:0"`
	PlatformFeeMinMicros        int64  `json:"platform_fee_min_micros" gorm:"default:0"`
	AuthorShareRatePPM          int64  `json:"author_share_rate_ppm" gorm:"default:0"`
	RelayPricePerGBMicros       int64  `json:"relay_price_per_gb_micros" gorm:"default:0"`
	StoragePricePerGBHourMicros int64  `json:"storage_price_per_gb_hour_micros" gorm:"default:0"`
	RiskReserveRatePPM          int64  `json:"risk_reserve_rate_ppm" gorm:"default:0"`
	FailurePolicy               string `json:"failure_policy" gorm:"type:varchar(16);default:full_refund"`
	RuleVersion                 string `json:"rule_version" gorm:"type:varchar(32)"`
	CreatedAt                   int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (PricingTemplate) TableName() string { return "pricing_templates" }

// ErrPricingTemplateNotFound is returned when a template id is missing.
var ErrPricingTemplateNotFound = errors.New("pricing template not found")

// ToPricingTemplate converts the persisted row into the pure pricing.Template.
func (t *PricingTemplate) ToPricingTemplate() pricing.Template {
	return pricing.Template{
		Currency:                    t.Currency,
		ProviderPriceMode:           t.ProviderPriceMode,
		PlatformFeeRatePPM:          t.PlatformFeeRatePPM,
		PlatformFeeMinMicros:        t.PlatformFeeMinMicros,
		AuthorShareRatePPM:          t.AuthorShareRatePPM,
		RelayPricePerGBMicros:       t.RelayPricePerGBMicros,
		StoragePricePerGBHourMicros: t.StoragePricePerGBHourMicros,
		RiskReserveRatePPM:          t.RiskReserveRatePPM,
		FailurePolicy:               t.FailurePolicy,
		RuleVersion:                 t.RuleVersion,
	}
}

// CreatePricingTemplate inserts a new immutable template.
func CreatePricingTemplate(t *PricingTemplate) error {
	return DB.Create(t).Error
}

// GetPricingTemplate returns a template by id.
func GetPricingTemplate(id int) (*PricingTemplate, error) {
	var t PricingTemplate
	if err := DB.Where("id = ?", id).First(&t).Error; err != nil {
		return nil, ErrPricingTemplateNotFound
	}
	return &t, nil
}
