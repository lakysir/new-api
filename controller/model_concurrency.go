package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetModelConcurrencyRules 返回并发规则列表，可通过 ?model=xxx 过滤。
// 同时回填用户名与当前进行中的任务数，供管理端展示「当前占用 / 上限」。
func GetModelConcurrencyRules(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	rules, err := model.GetModelConcurrencyRules(modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	counts, err := model.GetUnfinishedTaskCounts()
	if err != nil {
		// 占用数只是展示信息，查询失败时不影响规则列表本身
		common.SysError("get unfinished task counts error: " + err.Error())
		counts = map[int]map[string]int{}
	}

	userIds := make([]int, 0, len(rules))
	for _, rule := range rules {
		if rule.UserId != model.ModelConcurrencyAllUsers {
			userIds = append(userIds, rule.UserId)
		}
	}
	usernames, err := model.GetUsernamesByIds(userIds)
	if err != nil {
		common.SysError("get usernames for concurrency rules error: " + err.Error())
		usernames = map[int]string{}
	}

	for _, rule := range rules {
		if rule.UserId == model.ModelConcurrencyAllUsers {
			continue
		}
		rule.Username = usernames[rule.UserId]
		rule.Current = counts[rule.UserId][rule.ModelName]
	}

	common.ApiSuccess(c, rules)
}

type modelConcurrencyRuleRequest struct {
	ModelName      string `json:"model_name"`
	UserId         int    `json:"user_id"`
	MaxConcurrency int    `json:"max_concurrency"`
}

// UpsertModelConcurrencyRule 新增或更新一条并发规则。
// user_id 为 0 表示该模型对所有用户生效的默认规则；max_concurrency 为 0 表示不限制。
func UpsertModelConcurrencyRule(c *gin.Context) {
	var req modelConcurrencyRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	req.ModelName = strings.TrimSpace(req.ModelName)
	if req.ModelName == "" {
		common.ApiErrorMsg(c, "模型名称不能为空")
		return
	}
	if req.UserId < 0 {
		common.ApiErrorMsg(c, "无效的用户 ID")
		return
	}
	if req.MaxConcurrency < 0 {
		common.ApiErrorMsg(c, "并发上限不能为负数")
		return
	}
	if req.UserId != model.ModelConcurrencyAllUsers {
		if _, err := model.GetUserById(req.UserId, false); err != nil {
			common.ApiErrorMsg(c, "用户不存在")
			return
		}
	}

	rule, err := model.UpsertModelConcurrencyRule(req.ModelName, req.UserId, req.MaxConcurrency)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

// DeleteModelConcurrencyRule 删除一条并发规则。
func DeleteModelConcurrencyRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteModelConcurrencyRule(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// GetModelConcurrencyCandidateModels 返回下拉框候选模型名：历史异步任务中出现过的
// 模型，加上已配置规则的模型。管理端也允许手输任意模型名。
func GetModelConcurrencyCandidateModels(c *gin.Context) {
	names, err := model.GetTaskModelNames(200)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	rules, err := model.GetModelConcurrencyRules("")
	if err != nil {
		common.ApiError(c, err)
		return
	}

	seen := make(map[string]bool, len(names))
	candidates := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		candidates = append(candidates, name)
	}
	for _, rule := range rules {
		if seen[rule.ModelName] {
			continue
		}
		seen[rule.ModelName] = true
		candidates = append(candidates, rule.ModelName)
	}

	common.ApiSuccess(c, candidates)
}
