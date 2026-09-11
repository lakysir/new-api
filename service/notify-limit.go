package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/bytedance/gopkg/util/gopool"
)

// notifyLimitStore is used for in-memory rate limiting when Redis is disabled
var (
	notifyLimitStore sync.Map
	cleanupOnce      sync.Once
)

type limitCount struct {
	Count     int
	Timestamp time.Time
}

const quotaNotificationInterval = 8 * time.Hour

func getDuration(notifyType string) time.Duration {
	if notifyType == dto.NotifyTypeQuotaExceed {
		return quotaNotificationInterval
	}
	minute := constant.NotificationLimitDurationMinute
	return time.Duration(minute) * time.Minute
}

func getLimit(notifyType string) int {
	if notifyType == dto.NotifyTypeQuotaExceed {
		return 1
	}
	return constant.NotifyLimitCount
}

// startCleanupTask starts a background task to clean up expired entries
func startCleanupTask() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Hour)
			now := time.Now()
			notifyLimitStore.Range(func(key, value interface{}) bool {
				if limit, ok := value.(limitCount); ok {
					duration := getDuration("general")
					keyString, keyIsString := key.(string)
					if keyIsString && strings.HasSuffix(keyString, ":"+dto.NotifyTypeQuotaExceed) {
						duration = getDuration(dto.NotifyTypeQuotaExceed)
					}
					if now.Sub(limit.Timestamp) >= duration {
						notifyLimitStore.Delete(key)
					}
				}
				return true
			})
		}
	})
}

// CheckNotificationLimit checks if the user has exceeded their notification limit
// Returns true if the user can send notification, false if limit exceeded
func CheckNotificationLimit(userId int, notifyType string) (bool, error) {
	if common.RedisEnabled {
		return checkRedisLimit(userId, notifyType)
	}
	return checkMemoryLimit(userId, notifyType)
}

func checkRedisLimit(userId int, notifyType string) (bool, error) {
	key := fmt.Sprintf("notify_limit:%d:%s", userId, notifyType)
	duration := getDuration(notifyType)
	limit := getLimit(notifyType)
	ctx := context.Background()

	// Quota warnings are a single notification per interval. SETNX makes the
	// decision atomic when multiple requests finish at the same time.
	if notifyType == dto.NotifyTypeQuotaExceed {
		ok, err := common.RDB.SetNX(ctx, key, "1", duration).Result()
		if err != nil {
			return false, fmt.Errorf("failed to set notification limit: %w", err)
		}
		return ok, nil
	}

	// Get current count
	count, err := common.RedisGet(key)
	if err != nil && err.Error() != "redis: nil" {
		return false, fmt.Errorf("failed to get notification count: %w", err)
	}

	// If key doesn't exist, initialize it
	if count == "" {
		err = common.RedisSet(key, "1", duration)
		return true, err
	}

	currentCount, _ := strconv.Atoi(count)

	// Check if limit is already reached
	if currentCount >= limit {
		return false, nil
	}

	// Only increment if under limit
	err = common.RedisIncr(key, 1)
	if err != nil {
		return false, fmt.Errorf("failed to increment notification count: %w", err)
	}

	return true, nil
}

func checkMemoryLimit(userId int, notifyType string) (bool, error) {
	// Ensure cleanup task is started
	cleanupOnce.Do(startCleanupTask)

	key := fmt.Sprintf("%d:%s", userId, notifyType)
	now := time.Now()
	duration := getDuration(notifyType)

	// Get current limit count or initialize new one
	var currentLimit limitCount
	if value, ok := notifyLimitStore.Load(key); ok {
		currentLimit = value.(limitCount)
		// Check if the entry has expired
		if now.Sub(currentLimit.Timestamp) >= duration {
			currentLimit = limitCount{Count: 0, Timestamp: now}
		}
	} else {
		currentLimit = limitCount{Count: 0, Timestamp: now}
	}

	// Increment count
	currentLimit.Count++

	// Check against limits
	limit := getLimit(notifyType)

	// Store updated count
	notifyLimitStore.Store(key, currentLimit)

	return currentLimit.Count <= limit, nil
}
