package service

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const ModelConcurrencyReserveTTL = 5 * time.Minute
const ConcurrencyLimitReachedCode = "model_concurrency_limit_reached"
const UserConcurrencyLimitReachedCode = "user_concurrency_limit_reached"
const UserConcurrencyNotAllowedCode = "user_concurrency_not_allowed"
const ModelNotAllowedCode = "model_not_allowed"

var (
	concurrencyReserver     *limiter.ConcurrencyReserver
	concurrencyReserverOnce sync.Once
)

func getConcurrencyReserver() *limiter.ConcurrencyReserver {
	concurrencyReserverOnce.Do(func() {
		if common.RedisEnabled { concurrencyReserver = limiter.NewConcurrencyReserver(common.RDB) } else { concurrencyReserver = limiter.NewConcurrencyReserver(nil) }
	})
	return concurrencyReserver
}

func concurrencyReserveKey(userId int, modelName string) string { return fmt.Sprintf("concurrency:reserve:%d:%s", userId, modelName) }
func userConcurrencyReserveKey(userId int) string { return fmt.Sprintf("concurrency:reserve:%d:all", userId) }

// AcquireModelConcurrency applies both the per-model and per-user async limits.
func AcquireModelConcurrency(c *gin.Context, userId int, modelName string) (release func(), taskErr *dto.TaskError) {
	release = func() {}
	modelName = strings.TrimSpace(modelName)
	if userId <= 0 || modelName == "" { return release, nil }

	maxConcurrency := model.GetModelConcurrencyLimit(userId, modelName)
	if maxConcurrency <= model.ModelConcurrencyBlocked {
		message := i18n.T(c, i18n.MsgModelNotAllowed, map[string]any{"Model": modelName})
		return release, TaskErrorWrapperLocal(fmt.Errorf("%s", message), ModelNotAllowedCode, http.StatusForbidden)
	}
	member := c.GetString(common.RequestIdKey)
	if member == "" { member = common.NewRequestId() }
	reserver := getConcurrencyReserver()
	modelKey := concurrencyReserveKey(userId, modelName)
	modelReserved := false
	if maxConcurrency > 0 {
		dbCount, err := model.CountUnfinishedTaskByUserModel(userId, modelName)
		if err != nil { logger.LogError(c, fmt.Sprintf("count unfinished task for concurrency limit failed: %s", err.Error())); return release, nil }
		allowed, used, err := reserver.Reserve(c, modelKey, member, dbCount, maxConcurrency, ModelConcurrencyReserveTTL)
		if err != nil { logger.LogError(c, fmt.Sprintf("reserve model concurrency slot failed: %s", err.Error())); return release, nil }
		if !allowed {
			message := i18n.T(c, i18n.MsgModelConcurrencyLimitReached, map[string]any{"Model": modelName, "Max": maxConcurrency, "Current": used})
			return release, TaskErrorWrapperLocal(fmt.Errorf("%s", message), ConcurrencyLimitReachedCode, http.StatusTooManyRequests)
		}
		modelReserved = true
	}

	userMaxConcurrency := model.GetUserAsyncConcurrencyLimit(userId)
	userKey := userConcurrencyReserveKey(userId)
	userReserved := false
	if userMaxConcurrency <= model.ModelConcurrencyBlocked {
		if modelReserved {
			reserver.Release(c, modelKey, member)
		}
		message := i18n.T(c, i18n.MsgUserConcurrencyNotAllowed, nil)
		return release, TaskErrorWrapperLocal(fmt.Errorf("%s", message), UserConcurrencyNotAllowedCode, http.StatusForbidden)
	}
	if userMaxConcurrency > 0 {
		dbCount, err := model.CountUnfinishedTaskByUser(userId)
		if err != nil { logger.LogError(c, fmt.Sprintf("count unfinished task for user concurrency limit failed: %s", err.Error())); if modelReserved { reserver.Release(c, modelKey, member) }; return release, nil }
		allowed, used, err := reserver.Reserve(c, userKey, member, dbCount, userMaxConcurrency, ModelConcurrencyReserveTTL)
		if err != nil { logger.LogError(c, fmt.Sprintf("reserve user concurrency slot failed: %s", err.Error())); if modelReserved { reserver.Release(c, modelKey, member) }; return release, nil }
		if !allowed {
			if modelReserved { reserver.Release(c, modelKey, member) }
			message := i18n.T(c, i18n.MsgUserConcurrencyLimitReached, map[string]any{"Max": userMaxConcurrency, "Current": used})
			return release, TaskErrorWrapperLocal(fmt.Errorf("%s", message), UserConcurrencyLimitReachedCode, http.StatusTooManyRequests)
		}
		userReserved = true
	}

	released := false
	return func() {
		if released { return }
		released = true
		if modelReserved { reserver.Release(c, modelKey, member) }
		if userReserved { reserver.Release(c, userKey, member) }
	}, nil
}
