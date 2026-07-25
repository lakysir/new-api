package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// ModelConcurrency 限制单个用户在单个模型上同时进行中的异步任务数量。
//
// UserId == ModelConcurrencyAllUsers(0) 表示该模型对所有用户生效的默认规则；
// UserId > 0 表示针对指定用户的规则，优先级高于默认规则（不叠加，命中即生效）。
// MaxConcurrency == 0 表示不限制。
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

// GetModelConcurrencyLimit 返回用户在指定模型上的并发上限。
// 解析顺序：(模型, 该用户) → (模型, 所有用户) → 不限制。
// 返回 0 表示不限制。
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

// GetTaskModelNames 返回历史异步任务中出现过的模型名，供管理端下拉框使用。
func GetTaskModelNames(limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	var names []string
	err := DB.Model(&Task{}).
		Distinct().
		Where("model_name != ?", "").
		Order("model_name asc").
		Limit(limit).
		Pluck("model_name", &names).Error
	if err != nil {
		return nil, err
	}
	return names, nil
}
