package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupConcurrencyTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&ModelConcurrency{}, &UserAsyncConcurrency{}, &Task{}, &Model{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&ModelConcurrency{}).Error)
	require.NoError(t, DB.Where("1 = 1").Delete(&UserAsyncConcurrency{}).Error)
	require.NoError(t, DB.Where("1 = 1").Delete(&Task{}).Error)
	require.NoError(t, DB.Where("1 = 1").Delete(&Model{}).Error)
}

func TestUserAsyncConcurrencyRuleAndTotalCount(t *testing.T) {
	setupConcurrencyTables(t)
	require.NoError(t, DB.Create(&Task{UserId: 7, ModelName: "sora-2", Status: TaskStatusInProgress}).Error)
	require.NoError(t, DB.Create(&Task{UserId: 7, ModelName: "kling-v1", Status: TaskStatusQueued}).Error)
	require.NoError(t, DB.Create(&Task{UserId: 7, ModelName: "sora-2", Status: TaskStatusSuccess}).Error)

	_, err := UpsertUserAsyncConcurrencyRule(7, 30)
	require.NoError(t, err)
	assert.Equal(t, 30, GetUserAsyncConcurrencyLimit(7))
	count, err := CountUnfinishedTaskByUser(7)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

// 下拉框为空的根因回归：tasks 表为空时，模型库中的异步端点模型仍必须出现，
// 同时同步模型（如 gpt-4o）不能出现。
func TestGetAsyncModelNamesMergesAllSources(t *testing.T) {
	setupConcurrencyTables(t)

	require.NoError(t, DB.Create(&Model{
		ModelName: "sora-2",
		Endpoints: `["openai-video"]`,
	}).Error)
	require.NoError(t, DB.Create(&Model{
		ModelName: "gpt-4o",
		Endpoints: `["openai"]`,
	}).Error)

	// 历史任务里的模型，即便不在模型库中也要保留
	require.NoError(t, DB.Create(&Task{
		UserId:    1,
		ModelName: "kling-v1",
		Status:    TaskStatusSuccess,
	}).Error)

	// 已配置规则的模型不能从下拉框消失
	_, err := UpsertModelConcurrencyRule("vidu-1", ModelConcurrencyAllUsers, 3)
	require.NoError(t, err)

	names, err := GetAsyncModelNames()
	require.NoError(t, err)

	assert.Equal(t, []string{"kling-v1", "sora-2", "vidu-1"}, names)
	assert.NotContains(t, names, "gpt-4o")
}

// tasks 表完全为空时（新部署场景）下拉框也必须有内容。
func TestGetAsyncModelNamesWorksWithEmptyTaskTable(t *testing.T) {
	setupConcurrencyTables(t)

	require.NoError(t, DB.Create(&Model{
		ModelName: "sora-2",
		Endpoints: `["openai-video"]`,
	}).Error)

	names, err := GetAsyncModelNames()
	require.NoError(t, err)
	assert.Equal(t, []string{"sora-2"}, names)
}

// 优先级：(模型, 该用户) > (模型, 所有用户) > 不限制。
func TestGetModelConcurrencyLimitPriority(t *testing.T) {
	setupConcurrencyTables(t)

	assert.Equal(t, 0, GetModelConcurrencyLimit(7, "sora-2"))

	_, err := UpsertModelConcurrencyRule("sora-2", ModelConcurrencyAllUsers, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, GetModelConcurrencyLimit(7, "sora-2"))

	_, err = UpsertModelConcurrencyRule("sora-2", 7, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, GetModelConcurrencyLimit(7, "sora-2"))

	// 用户级填 0 表示该用户不限制，必须覆盖掉所有用户的 2
	_, err = UpsertModelConcurrencyRule("sora-2", 7, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, GetModelConcurrencyLimit(7, "sora-2"))

	// 其他用户仍走所有用户规则
	assert.Equal(t, 2, GetModelConcurrencyLimit(8, "sora-2"))
}

// -1（禁止使用）也要遵循同样的优先级：所有用户禁止 + 指定用户放开 = 白名单。
func TestGetModelConcurrencyLimitBlocked(t *testing.T) {
	setupConcurrencyTables(t)

	_, err := UpsertModelConcurrencyRule("sora-2", ModelConcurrencyAllUsers, ModelConcurrencyBlocked)
	require.NoError(t, err)
	assert.Equal(t, ModelConcurrencyBlocked, GetModelConcurrencyLimit(7, "sora-2"))

	// 白名单用户：指定用户规则覆盖掉「所有用户禁止」
	_, err = UpsertModelConcurrencyRule("sora-2", 7, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, GetModelConcurrencyLimit(7, "sora-2"))
	assert.Equal(t, ModelConcurrencyBlocked, GetModelConcurrencyLimit(8, "sora-2"))

	// 反向：所有用户放开，单独禁掉某个用户
	_, err = UpsertModelConcurrencyRule("kling-v1", ModelConcurrencyAllUsers, 0)
	require.NoError(t, err)
	_, err = UpsertModelConcurrencyRule("kling-v1", 9, ModelConcurrencyBlocked)
	require.NoError(t, err)
	assert.Equal(t, 0, GetModelConcurrencyLimit(8, "kling-v1"))
	assert.Equal(t, ModelConcurrencyBlocked, GetModelConcurrencyLimit(9, "kling-v1"))
}

// 「移除该模型的并发配置」必须连带删掉指定用户规则，且不影响其他模型。
func TestDeleteModelConcurrencyRulesByModel(t *testing.T) {
	setupConcurrencyTables(t)

	_, err := UpsertModelConcurrencyRule("sora-2", ModelConcurrencyAllUsers, 2)
	require.NoError(t, err)
	_, err = UpsertModelConcurrencyRule("sora-2", 7, 5)
	require.NoError(t, err)
	_, err = UpsertModelConcurrencyRule("kling-v1", ModelConcurrencyAllUsers, 1)
	require.NoError(t, err)

	deleted, err := DeleteModelConcurrencyRulesByModel("sora-2")
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	// 删完之后该模型回到不限制，其他模型的规则保持不变
	assert.Equal(t, 0, GetModelConcurrencyLimit(7, "sora-2"))
	assert.Equal(t, 1, GetModelConcurrencyLimit(7, "kling-v1"))

	remaining, err := GetModelConcurrencyRules("")
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "kling-v1", remaining[0].ModelName)

	// 空模型名不应该误删任何数据
	deleted, err = DeleteModelConcurrencyRulesByModel("  ")
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

// 只统计进行中的任务，且严格按 (用户, 模型) 隔离。
func TestCountUnfinishedTaskByUserModel(t *testing.T) {
	setupConcurrencyTables(t)

	rows := []Task{
		{UserId: 1, ModelName: "sora-2", Status: TaskStatusInProgress},
		{UserId: 1, ModelName: "sora-2", Status: TaskStatusQueued},
		{UserId: 1, ModelName: "sora-2", Status: TaskStatusSuccess},
		{UserId: 1, ModelName: "sora-2", Status: TaskStatusFailure},
		{UserId: 1, ModelName: "kling-v1", Status: TaskStatusInProgress},
		{UserId: 2, ModelName: "sora-2", Status: TaskStatusInProgress},
	}
	for i := range rows {
		require.NoError(t, DB.Create(&rows[i]).Error)
	}

	count, err := CountUnfinishedTaskByUserModel(1, "sora-2")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}
