package sora

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

func TestAdjustBillingOnSubmitAddsMultiplierAndPreservesRatios(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{OtherRatios: map[string]float64{
			"seconds": 6,
			"size":    1.666667,
		}},
	}

	ratios := (&TaskAdaptor{}).AdjustBillingOnSubmit(info, []byte(`{
		"id":"upstream-task",
		"status":"queued",
		"billing_multiplier":2.5
	}`))

	if ratios == nil {
		t.Fatal("expected adjusted ratios")
	}
	if ratios["seconds"] != 6 || ratios["size"] != 1.666667 || ratios["billing_multiplier"] != 2.5 {
		t.Fatalf("unexpected ratios: %#v", ratios)
	}
	if info.PriceData.OtherRatios["billing_multiplier"] != 0 {
		t.Fatal("adjustment must not mutate the original ratios")
	}
}

func TestAdjustBillingOnSubmitIgnoresMissingOrInvalidMultiplier(t *testing.T) {
	tests := []string{
		`{"id":"task"}`,
		`{"id":"task","billing_multiplier":0}`,
		`{"id":"task","billing_multiplier":-1}`,
		`{"id":"task","billing_multiplier":"2.5"}`,
		`{"id":"task","billing_multiplier":null}`,
	}

	for _, body := range tests {
		info := &relaycommon.RelayInfo{
			PriceData: types.PriceData{OtherRatios: map[string]float64{"seconds": 6}},
		}
		if ratios := (&TaskAdaptor{}).AdjustBillingOnSubmit(info, []byte(body)); ratios != nil {
			t.Fatalf("expected no adjustment for %s, got %#v", body, ratios)
		}
	}
}
