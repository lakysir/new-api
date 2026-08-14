package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDoubaoVideoTaskResponsePreservesSuccessfulUsage(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_public",
		ModelName: "doubao-seedance-2-0-260128",
		Status:    model.TaskStatusSuccess,
		CreatedAt: 100,
		UpdatedAt: 200,
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://example.com/result.mp4",
		},
		Data: json.RawMessage(`{
			"id":"upstream_secret",
			"status":"succeeded",
			"duration":6,
			"content":{"video_url":"https://upstream.example/result.mp4"},
			"usage":{"completion_tokens":1200,"total_tokens":1500}
		}`),
	}

	body, err := buildDoubaoVideoTaskResponse(task)
	require.NoError(t, err)
	var response struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response.ID)
	assert.Equal(t, "doubao-seedance-2-0-260128", response.Model)
	assert.Equal(t, "succeeded", response.Status)
	assert.Equal(t, "https://example.com/result.mp4", response.Content.VideoURL)
	assert.Equal(t, 1200, response.Usage.CompletionTokens)
	assert.Equal(t, 1500, response.Usage.TotalTokens)
}

func TestBuildDoubaoVideoTaskResponseMapsFailure(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_failed",
		ModelName:  "doubao-seedance-2-0-260128",
		Status:     model.TaskStatusFailure,
		FailReason: "upstream rejected the task",
	}

	body, err := buildDoubaoVideoTaskResponse(task)
	require.NoError(t, err)
	var response struct {
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "failed", response.Status)
	assert.Equal(t, "task_failed", response.Error.Code)
	assert.Equal(t, "upstream rejected the task", response.Error.Message)
}
