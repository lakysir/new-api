package sora

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultReadsUsageOnSuccess(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id":"upstream-task",
		"status":"completed",
		"usage":{"completion_tokens":1200,"total_tokens":1500}
	}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, 1200, result.CompletionTokens)
	assert.Equal(t, 1500, result.TotalTokens)
}

func TestParseTaskResultIgnoresSubmitBillingMultiplier(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id":"upstream-task",
		"status":"completed",
		"billing_multiplier":2.5
	}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Zero(t, result.TotalTokens)
}
