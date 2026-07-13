package pricing

import "testing"

func baseTemplate() Template {
	return Template{
		Currency:           "USD",
		ProviderPriceMode:  ModePerTask,
		PlatformFeeRatePPM: 80000, // 8%
		AuthorShareRatePPM: 30000, // 3%
		RiskReserveRatePPM: 10000, // 1%
		FailurePolicy:      FailureFullRefund,
		RuleVersion:        "v1",
	}
}

func TestComputeBreakdownSumEqualsMax(t *testing.T) {
	b, err := Compute(100000, baseTemplate(), Estimate{})
	if err != nil {
		t.Fatal(err)
	}
	if b.SumComponents() != b.MaxCustomerMicros {
		t.Fatalf("components %d must equal max %d", b.SumComponents(), b.MaxCustomerMicros)
	}
	// provider 100000 + author 3000 + platform 8000 + risk 1000 = 112000
	if b.MaxCustomerMicros != 112000 {
		t.Fatalf("expected 112000, got %d", b.MaxCustomerMicros)
	}
	if b.ProviderMicros != 100000 || b.AuthorMicros != 3000 || b.PlatformFeeMicros != 8000 || b.RiskReserveMicros != 1000 {
		t.Fatalf("unexpected breakdown: %+v", b)
	}
}

func TestPlatformFeeMinimumApplies(t *testing.T) {
	tpl := baseTemplate()
	tpl.PlatformFeeMinMicros = 5000
	// 8% of 10000 = 800, below the 5000 floor.
	b, _ := Compute(10000, tpl, Estimate{})
	if b.PlatformFeeMicros != 5000 {
		t.Fatalf("expected floor 5000, got %d", b.PlatformFeeMicros)
	}
}

func TestRelayAndStorageReservedFromEstimate(t *testing.T) {
	tpl := baseTemplate()
	tpl.RelayPricePerGBMicros = 120000
	tpl.StoragePricePerGBHourMicros = 20000
	b, _ := Compute(0, tpl, Estimate{RelayGB: 0.5, StorageGBHours: 2})
	// relay: ceil(0.5*120000)=60000, storage: ceil(2*20000)=40000
	if b.RelayFeeMicros != 60000 || b.StorageFeeMicros != 40000 {
		t.Fatalf("relay/storage wrong: %+v", b)
	}
}

func TestCeilRoundsUpFractionalUsage(t *testing.T) {
	tpl := baseTemplate()
	tpl.RelayPricePerGBMicros = 3                   // tiny price to force fractional micros
	b, _ := Compute(0, tpl, Estimate{RelayGB: 0.5}) // 0.5*3 = 1.5 -> ceil 2
	if b.RelayFeeMicros != 2 {
		t.Fatalf("expected ceil to 2, got %d", b.RelayFeeMicros)
	}
}

func TestZeroBidMicroTask(t *testing.T) {
	// A near-zero micro-task: provider bid 100000 micros = 0.1 USD.
	b, err := Compute(100000, baseTemplate(), Estimate{})
	if err != nil {
		t.Fatal(err)
	}
	if b.SumComponents() != b.MaxCustomerMicros {
		t.Fatal("invariant broken for micro task")
	}
}

func TestNegativeBidRejected(t *testing.T) {
	if _, err := Compute(-1, baseTemplate(), Estimate{}); err != ErrNegativeBid {
		t.Fatalf("expected ErrNegativeBid, got %v", err)
	}
}

func TestBadRateRejected(t *testing.T) {
	tpl := baseTemplate()
	tpl.PlatformFeeRatePPM = PPMDenominator + 1
	if _, err := Compute(1000, tpl, Estimate{}); err != ErrBadRate {
		t.Fatalf("expected ErrBadRate, got %v", err)
	}
}
