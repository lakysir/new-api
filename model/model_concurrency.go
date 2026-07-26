package model

import (
	"slices"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

// ModelConcurrency 限制单个用户在单个模型上同时进行中的异步任务数量。
//
// UserId == ModelConcurrencyAllUsers(0) 表示该模型对所有用户生效的默认规则；
// UserId > 0 表示针对指定用户的规则，优先级高于默认规则（不叠加，命中即生效）。
// MaxConcurrency == 0 表示不限制；== ModelConcurrencyBlocked(-1) 表示禁止提交。
type ModelConcurrency struct {
	Id             int    `json:"id"`
	ModelName      string `json:"model_name" gorm:"size:128;not null;uniqueIndex:uk_model_concurrency,priority:1"`
	UserId         int    `json:"user_id" gorm:"not null;default:0;uniqueIndex:uk_model_concurrency,priority:2"`
	MaxConcurrency int    `json:"max_concurrency" gorm:"not null;default:0"`
	CreatedTime    int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime    int64  `json:"updated_time" gorm:"bigint"`

	Username string `json:"username,omitempty" gorm:"-"`
	Current  int    `json:"current" gorm:"-"`
}

// ModelConcurrencyAllUsers 是「所有用户」规则使用的 UserId 取值。
const ModelConcurrencyAllUsers = 0

// ModelConcurrencyBlocked 是 MaxConcurrency 的特殊取值：完全禁止该用户使用该模型。
// 配合「所有用户 = -1 + 指定用户 = 0/N」即可做到只放开白名单用户。
const ModelConcurrencyBlocked = -1

// GetModelConcurrencyLimit 返回用户在指定模型上的并发上限。
// 解析顺序：(模型, 该用户) → (模型, 所有用户) → 不限制。
// 返回 0 表示不限制，返回 ModelConcurrencyBlocked(-1) 表示禁止提交。
func GetModelConcurrencyLimit(userId int, modelName string) int {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || userId <= 0 {
		return 0
	}

	var rules []ModelConcurrency
	err := DB.Where("model_name = ?", modelName).
		Where("user_id IN (?, ?)", userId, ModelConcurrencyAllUsers).
		Find(&rules).Error
	if err != nil {
		common.SysError("query model concurrency rule error: " + err.Error())
		return 0
	}

	limit := 0
	for _, rule := range rules {
		if rule.UserId == userId {
			return rule.MaxConcurrency
		}
		limit = rule.MaxConcurrency
	}
	return limit
}

// CountUnfinishedTaskByUserModel 统计用户在指定模型上仍在进行中的异步任务数量。
// 「进行中」指状态既不是 SUCCESS 也不是 FAILURE。
func CountUnfinishedTaskByUserModel(userId int, modelName string) (int64, error) {
	var total int64
	err := DB.Model(&Task{}).
		Where("user_id = ?", userId).
		Where("model_name = ?", modelName).
		Where("status != ?", TaskStatusSuccess).
		Where("status != ?", TaskStatusFailure).
		Count(&total).Error
	return total, err
}

type unfinishedTaskGroup struct {
	UserId    int    `gorm:"column:user_id"`
	ModelName string `gorm:"column:model_name"`
	Total     int    `gorm:"column:total"`
}

// GetUnfinishedTaskCounts 返回所有 (用户, 模型) 维度上进行中的异步任务数量，
// 供管理端展示「当前占用 / 上限」。
func GetUnfinishedTaskCounts() (map[int]map[string]int, error) {
	var rows []unfinishedTaskGroup
	err := DB.Model(&Task{}).
		Select("user_id, model_name, count(*) as total").
		Where("status != ?", TaskStatusSuccess).
		Where("status != ?", TaskStatusFailure).
		Group("user_id, model_name").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[int]map[string]int, len(rows))
	for _, row := range rows {
		if counts[row.UserId] == nil {
			counts[row.UserId] = make(map[string]int)
		}
		counts[row.UserId][row.ModelName] = row.Total
	}
	return counts, nil
}

// GetModelConcurrencyRules 返回并发规则列表，modelName 为空时返回全部。
func GetModelConcurrencyRules(modelName string) ([]*ModelConcurrency, error) {
	var rules []*ModelConcurrency
	query := DB.Model(&ModelConcurrency{})
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	if err := query.Order("model_name asc, user_id asc").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// UpsertModelConcurrencyRule 按 (模型, 用户) 新增或更新规则。
func UpsertModelConcurrencyRule(modelName string, userId int, maxConcurrency int) (*ModelConcurrency, error) {
	now := common.GetTimestamp()
	var rule ModelConcurrency
	err := DB.Where("model_name = ? AND user_id = ?", modelName, userId).First(&rule).Error
	if err == nil {
		rule.MaxConcurrency = maxConcurrency
		rule.UpdatedTime = now
		if err := DB.Save(&rule).Error; err != nil {
			return nil, err
		}
		return &rule, nil
	}

	rule = ModelConcurrency{
		ModelName:      modelName,
		UserId:         userId,
		MaxConcurrency: maxConcurrency,
		CreatedTime:    now,
		UpdatedTime:    now,
	}
	if err := DB.Create(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// DeleteModelConcurrencyRule 删除指定规则。
func DeleteModelConcurrencyRule(id int) error {
	return DB.Delete(&ModelConcurrency{}, id).Error
}

// DeleteModelConcurrencyRulesByModel 删除某个模型下的全部规则（含「所有用户」与各指定用户）。
// 管理端「移除该模型的并发配置」用这个；删完之后该模型即回到不限制状态。
func DeleteModelConcurrencyRulesByModel(modelName string) (int64, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return 0, nil
	}
	tx := DB.Where("model_name = ?", modelName).Delete(&ModelConcurrency{})
	return tx.RowsAffected, tx.Error
}

// asyncEndpointTypes 是会产生异步任务的端点类型。只有这些端点的模型才需要并发限制。
// 部分类型在 constant 中尚未定义常量（被注释），这里用字面量兜住；多余的值不会匹配到任何模型。
var asyncEndpointTypes = []string{
	string(constant.EndpointTypeOpenAIVideo),
	"video-generation",
	"suno-proxy",
	"kling",
	"jimeng",
	"midjourney-proxy",
}

// GetAsyncModelNames 返回可配置并发限制的模型名，合并三个来源：
//  1. 模型库中端点类型属于异步的模型 —— 保证新部署（tasks 表为空）时下拉框就有内容
//  2. 历史异步任务中出现过的模型
//  3. 已经配置过规则的模型 —— 避免规则存在但模型从下拉框消失
func GetAsyncModelNames() ([]string, error) {
	seen := make(map[string]bool)

	var libRows []struct {
		ModelName string
		Endpoints string
	}
	if err := DB.Model(&Model{}).
		Select("model_name, endpoints").
		Where("model_name IS NOT NULL AND model_name != ?", "").
		Find(&libRows).Error; err != nil {
		return nil, err
	}
	// JSON 包含查询在 SQLite/MySQL/PostgreSQL 上写法不通用，取回后在 Go 里过滤
	for _, row := range libRows {
		var endpoints []string
		if common.Unmarshal([]byte(row.Endpoints), &endpoints) != nil {
			continue
		}
		for _, endpoint := range endpoints {
			if slices.Contains(asyncEndpointTypes, endpoint) {
				seen[row.ModelName] = true
				break
			}
		}
	}

	// model_name 可能为 NULL：只写 != '' 会因 SQL 三值逻辑静默丢掉这些行。
	// Distinct() 与 Pluck 组合在不同驱动上行为不一致，改用 Distinct("model_name")。
	var taskNames []string
	if err := DB.Model(&Task{}).
		Distinct("model_name").
		Where("model_name IS NOT NULL AND model_name != ?", "").
		Pluck("model_name", &taskNames).Error; err != nil {
		return nil, err
	}
	for _, name := range taskNames {
		seen[name] = true
	}

	rules, err := GetModelConcurrencyRules("")
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		seen[rule.ModelName] = true
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
