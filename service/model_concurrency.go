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

// ModelConcurrencyReserveTTL 是预留位的存活时间。它只需覆盖「并发判定 → 上游提交
// → 任务落库」这一段，取值偏大一些以容忍上游响应缓慢（如带图片上传的视频提交），
// 同时保证异常情况下槽位最终会自动释放。
const ModelConcurrencyReserveTTL = 5 * time.Minute

// ConcurrencyLimitReachedCode 是并发已满时返回给客户端的错误码。
const ConcurrencyLimitReachedCode = "model_concurrency_limit_reached"

var (
	concurrencyReserver     *limiter.ConcurrencyReserver
	concurrencyReserverOnce sync.Once
)

func getConcurrencyReserver() *limiter.ConcurrencyReserver {
	concurrencyReserverOnce.Do(func() {
		if common.RedisEnabled {
			concurrencyReserver = limiter.NewConcurrencyReserver(common.RDB)
		} else {
			concurrencyReserver = limiter.NewConcurrencyReserver(nil)
		}
	})
	return concurrencyReserver
}

func concurrencyReserveKey(userId int, modelName string) string {
	return fmt.Sprintf("concurrency:reserve:%d:%s", userId, modelName)
}

// AcquireModelConcurrency 检查用户在指定模型上的异步任务并发是否已满。
//
// 返回的 release 函数必须在提交流程结束后调用（无论成功或失败）：任务已落库时
// 数据库计数已能反映它，提交失败时该次占位应当归还。release 恒不为 nil。
// 当未配置上限（0 表示不限制）时直接放行。
func AcquireModelConcurrency(c *gin.Context, userId int, modelName string) (release func(), taskErr *dto.TaskError) {
	release = func() {}

	modelName = strings.TrimSpace(modelName)
	if userId <= 0 || modelName == "" {
		return release, nil
	}

	maxConcurrency := model.GetModelConcurrencyLimit(userId, modelName)
	if maxConcurrency <= 0 {
		return release, nil
	}

	dbCount, err := model.CountUnfinishedTaskByUserModel(userId, modelName)
	if err != nil {
		// 计数失败时放行，避免数据库抖动导致所有任务提交被拒。
		logger.LogError(c, fmt.Sprintf("count unfinished task for concurrency limit failed: %s", err.Error()))
		return release, nil
	}

	key := concurrencyReserveKey(userId, modelName)
	member := c.GetString(common.RequestIdKey)
	if member == "" {
		member = common.NewRequestId()
	}

	reserver := getConcurrencyReserver()
	allowed, used, err := reserver.Reserve(c, key, member, dbCount, maxConcurrency, ModelConcurrencyReserveTTL)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("reserve model concurrency slot failed: %s", err.Error()))
		return release, nil
	}

	if !allowed {
		message := i18n.T(c, i18n.MsgModelConcurrencyLimitReached, map[string]any{
			"Model":   modelName,
			"Max":     maxConcurrency,
			"Current": used,
		})
		return release, TaskErrorWrapperLocal(fmt.Errorf("%s", message), ConcurrencyLimitReachedCode, http.StatusTooManyRequests)
	}

	released := false
	return func() {
		if released {
			return
		}
		released = true
		reserver.Release(c, key, member)
	}, nil
}
