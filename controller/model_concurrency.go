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

	// 「所有用户」规则没有具体用户，展示该模型上所有用户进行中的任务总数，
	// 让管理员在列表页一眼看到这个模型当前有多少任务在跑。
	modelTotals := map[string]int{}
	for _, perModel := range counts {
		for name, total := range perModel {
			modelTotals[name] += total
		}
	}

	for _, rule := range rules {
		if rule.UserId == model.ModelConcurrencyAllUsers {
			rule.Current = modelTotals[rule.ModelName]
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
// user_id 为 0 表示该模型对所有用户生效的默认规则；
// max_concurrency 为 0 表示不限制，-1 表示禁止该用户使用该模型。
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
	// -1 是「禁止使用该模型」的特殊取值，比它更小的负数无意义。
	if req.MaxConcurrency < model.ModelConcurrencyBlocked {
		common.ApiErrorMsg(c, "并发上限只能填 -1（禁止使用）、0（不限制）或正整数")
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

// DeleteModelConcurrencyRulesByModel 删除某个模型下的全部并发规则。
// 模型名走 query 参数而不是路径参数：模型名里可能带 '/'（如 provider/model），
// 放进路径会被 gin 的路由解析切断。
func DeleteModelConcurrencyRulesByModel(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" {
		common.ApiErrorMsg(c, "模型名称不能为空")
		return
	}
	deleted, err := model.DeleteModelConcurrencyRulesByModel(modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"deleted": deleted})
}

// GetModelConcurrencyCandidateModels 返回下拉框候选模型名：模型库中的异步端点模型、
// 历史异步任务中的模型、已配置规则的模型。管理端也允许手输任意模型名。
func GetModelConcurrencyCandidateModels(c *gin.Context) {
	names, err := model.GetAsyncModelNames()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, names)
}
