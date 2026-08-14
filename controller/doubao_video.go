package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func RelayDoubaoVideoTaskFetch(c *gin.Context) {
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("task_id"))
	if err != nil {
		respondTaskError(c, service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError))
		return
	}
	if !exists {
		respondTaskError(c, service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusNotFound))
		return
	}

	responseBody, err := buildDoubaoVideoTaskResponse(task)
	if err != nil {
		respondTaskError(c, service.TaskErrorWrapper(err, "build_task_response_failed", http.StatusInternalServerError))
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", responseBody)
}

func buildDoubaoVideoTaskResponse(task *model.Task) ([]byte, error) {
	response := map[string]any{}
	if len(task.Data) > 0 {
		_ = common.Unmarshal(task.Data, &response)
	}
	if response == nil {
		response = map[string]any{}
	}

	delete(response, "task_id")
	response["id"] = task.TaskID
	response["model"] = doubaoTaskModelName(task)
	response["status"] = doubaoTaskStatus(task.Status)
	response["created_at"] = task.CreatedAt
	response["updated_at"] = task.UpdatedAt

	if task.Status == model.TaskStatusSuccess {
		content, _ := response["content"].(map[string]any)
		if content == nil {
			content = map[string]any{}
		}
		if resultURL := task.GetResultURL(); resultURL != "" {
			content["video_url"] = resultURL
		}
		response["content"] = content
	}

	if task.Status == model.TaskStatusFailure {
		errorData, _ := response["error"].(map[string]any)
		if errorData == nil {
			errorData = map[string]any{}
		}
		if _, ok := errorData["code"]; !ok {
			errorData["code"] = "task_failed"
		}
		if task.FailReason != "" {
			errorData["message"] = task.FailReason
		} else if _, ok := errorData["message"]; !ok {
			errorData["message"] = "task failed"
		}
		response["error"] = errorData
	}

	return common.Marshal(response)
}

func doubaoTaskModelName(task *model.Task) string {
	if task.ModelName != "" {
		return task.ModelName
	}
	if task.Properties.OriginModelName != "" {
		return task.Properties.OriginModelName
	}
	return task.Properties.UpstreamModelName
}

func doubaoTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted, model.TaskStatusNotStart:
		return "queued"
	default:
		return "processing"
	}
}
