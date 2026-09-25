package ratio_setting

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type ReferenceImagePricing struct {
	FreeCount int     `json:"free_count"`
	UnitPrice float64 `json:"unit_price"`
}

var referenceImagePricing = map[string]ReferenceImagePricing{}
var referenceImagePricingMu sync.RWMutex

func ReferenceImagePricing2JSONString() string {
	referenceImagePricingMu.RLock()
	defer referenceImagePricingMu.RUnlock()
	b, _ := common.Marshal(referenceImagePricing)
	return string(b)
}

func UpdateReferenceImagePricingByJSONString(value string) error {
	next := map[string]ReferenceImagePricing{}
	if value != "" {
		if err := common.UnmarshalJsonStr(value, &next); err != nil { return err }
	}
	referenceImagePricingMu.Lock()
	referenceImagePricing = next
	referenceImagePricingMu.Unlock()
	return nil
}

func GetReferenceImagePricing(modelName string) (ReferenceImagePricing, bool) {
	referenceImagePricingMu.RLock()
	defer referenceImagePricingMu.RUnlock()
	v, ok := referenceImagePricing[modelName]
	return v, ok
}
