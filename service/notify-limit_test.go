package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
)

func TestCheckMemoryLimitQuotaWarningUsesEightHourInterval(t *testing.T) {
	key := "42:" + dto.NotifyTypeQuotaExceed
	notifyLimitStore.Delete(key)
	defer notifyLimitStore.Delete(key)

	allowed, err := checkMemoryLimit(42, dto.NotifyTypeQuotaExceed)
	if err != nil || !allowed {
		t.Fatalf("first quota warning allowed=%v err=%v, want allowed", allowed, err)
	}
	allowed, err = checkMemoryLimit(42, dto.NotifyTypeQuotaExceed)
	if err != nil || allowed {
		t.Fatalf("second quota warning allowed=%v err=%v, want suppressed", allowed, err)
	}

	notifyLimitStore.Store(key, limitCount{
		Count:     1,
		Timestamp: time.Now().Add(-quotaNotificationInterval - time.Minute),
	})
	allowed, err = checkMemoryLimit(42, dto.NotifyTypeQuotaExceed)
	if err != nil || !allowed {
		t.Fatalf("quota warning after interval allowed=%v err=%v, want allowed", allowed, err)
	}
}
