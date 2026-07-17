package marketplace

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/dispatch"
	"github.com/QuantumNous/new-api/service/nodehub"
	"github.com/QuantumNous/new-api/service/order"
	"github.com/QuantumNous/new-api/service/receipt"
	"github.com/QuantumNous/new-api/service/relayclient"
	"github.com/QuantumNous/new-api/service/settlement"

	"github.com/gin-gonic/gin"
)

// DoRequest creates and dispatches a marketplace order for the bound script,
// funded by the publishing operator, then launches the in-process E2EE session
// in the background. It returns a synthetic "queued" response immediately; the
// background executor fills in the result on the Task record for later fetch.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	bindingVal, ok := c.Get(ctxKeyBinding)
	if !ok {
		return nil, errors.New("marketplace binding missing on context")
	}
	binding, ok := bindingVal.(*model.ScriptModelBinding)
	if !ok || binding == nil {
		return nil, errors.New("marketplace binding invalid on context")
	}
	configVal, _ := c.Get(ctxKeyConfig)
	config, _ := configVal.(map[string]any)

	// Units of work: prefer the request's seconds, else the binding default.
	multiplier := binding.ConsumeMultiplier
	if seconds := extractSeconds(config); seconds > 0 {
		multiplier = int64(seconds)
	}
	if multiplier < 1 {
		multiplier = 1
	}

	// Create the order under the publishing operator (funds his balance). The
	// idempotency key is derived from the public task id so a submit retry
	// resolves to the same order instead of creating (or executing) it twice.
	idempotencyKey := fmt.Sprintf("bridge-%s", info.PublicTaskID)
	o, created, err := order.Create(order.CreateRequest{
		ClientId:          binding.PublisherUserId,
		ScriptId:          binding.ScriptId,
		Version:           binding.Version,
		InputHash:         inputHash(config),
		IdempotencyKey:    idempotencyKey,
		ConsumeMultiplier: multiplier,
	})
	if err != nil {
		return nil, fmt.Errorf("create order failed: %w", err)
	}

	// Only a freshly created order reserves funds, dispatches and launches the
	// E2EE session. A retry that resolves to an existing order just re-returns
	// the queued body (the original session is already running).
	if created && o.State == model.OrderCreated {
		o, err = settlement.ReserveFunds(o.Id)
		if err != nil {
			return nil, fmt.Errorf("reserve funds failed (operator marketplace balance?): %w", err)
		}
		result, dispatchErr := dispatch.Dispatch(o.Id, 1)
		if dispatchErr != nil && !errors.Is(dispatchErr, dispatch.ErrNoCandidates) {
			_, _ = settlement.Refund(o.Id)
			return nil, fmt.Errorf("dispatch failed: %w", dispatchErr)
		}
		if result != nil {
			if publishErr := dispatch.PublishEvent(nodehub.Default, result.EventId); publishErr != nil {
				if ta, _ := model.GetTaskAttempt(o.Id, 1); ta != nil {
					_ = model.ReleaseLease(ta.LeaseId, "offer_delivery_failed")
				}
				if current, _ := model.GetOrder(o.Id); current != nil && current.State == model.OrderOffered {
					_, _ = model.ApplyTransition(o.Id, model.OrderCancelled, nil)
					_, _ = settlement.Refund(o.Id)
				}
				return nil, errors.New("provider control channel unavailable; funds refunded")
			}
		}
		// Launch the E2EE session in the background; it settles the order and
		// writes the result onto the Task (inserted by the controller after
		// DoResponse returns).
		go a.executeSession(o.Id, binding.PublisherUserId, info.PublicTaskID, config)
	}

	c.Set(ctxKeyOrderID, o.Id)

	body, _ := common.Marshal(map[string]any{
		"id":     info.PublicTaskID,
		"object": "video",
		"status": dto.VideoStatusQueued,
		"model":  a.modelName,
	})
	return syntheticResponse(body), nil
}

// executeSession runs the client relay session and settles the order, then
// writes the OpenAI-video-shaped result onto the Task for later fetch.
func (a *TaskAdaptor) executeSession(orderID string, clientID int, publicTaskID string, config map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			common.SysError(fmt.Sprintf("marketplace executeSession panic: %v", r))
		}
	}()

	configBytes, _ := common.Marshal(config)
	result, err := relayclient.RunClientSession(orderID, clientID, configBytes)
	if err != nil {
		a.finalizeFailure(orderID, publicTaskID, err.Error())
		return
	}

	// Submit the client receipt so the control plane can reconcile with the
	// provider's receipt and settle the order.
	_ = model.SaveReceipt(&model.Receipt{
		TaskId:     orderID,
		Attempt:    1,
		Party:      receipt.PartyClient,
		OrderId:    orderID,
		ResultHash: resultHash(result),
	})
	if o, _ := model.GetOrder(orderID); o != nil && o.State == model.OrderRunning {
		_, _ = model.ApplyTransition(orderID, model.OrderResultReady, nil)
		_, _ = model.ApplyTransition(orderID, model.OrderVerifying, nil)
	}
	_, _ = settlement.ReconcileAndSettle(orderID, orderID, 1)

	a.finalizeSuccess(publicTaskID, result)
}

// finalizeSuccess writes the completed OpenAI video body onto the Task.
func (a *TaskAdaptor) finalizeSuccess(publicTaskID string, result []byte) {
	video := dto.NewOpenAIVideo()
	video.ID = publicTaskID
	video.Model = a.modelName
	video.Status = dto.VideoStatusCompleted
	video.Progress = 100
	video.CompletedAt = time.Now().Unix()
	if url := extractResultURL(result); url != "" {
		video.SetMetadata("url", url)
	}
	// Carry the raw script result so callers that inspect metadata get it all.
	video.SetMetadata("result", decodeResult(result))
	data, _ := common.Marshal(video)
	updateTaskWhenPresent(publicTaskID, map[string]any{
		"status":      model.TaskStatusSuccess,
		"progress":    "100%",
		"finish_time": time.Now().Unix(),
		"data":        string(data),
	})
}

// finalizeFailure marks the Task failed with the reason.
func (a *TaskAdaptor) finalizeFailure(orderID, publicTaskID, reason string) {
	video := dto.NewOpenAIVideo()
	video.ID = publicTaskID
	video.Model = a.modelName
	video.Status = dto.VideoStatusFailed
	video.Error = &dto.OpenAIVideoError{Message: reason, Code: "marketplace_execution_failed"}
	data, _ := common.Marshal(video)
	updateTaskWhenPresent(publicTaskID, map[string]any{
		"status":      model.TaskStatusFailure,
		"fail_reason": reason,
		"finish_time": time.Now().Unix(),
		"data":        string(data),
	})
}

// updateTaskWhenPresent polls briefly for the Task row (inserted by the
// controller after DoResponse) and applies the update. Bounded so a missing row
// never blocks forever.
func updateTaskWhenPresent(publicTaskID string, params map[string]any) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		task, exist, err := model.GetByOnlyTaskId(publicTaskID)
		if err == nil && exist && task != nil {
			if updErr := model.TaskBulkUpdate([]string{publicTaskID}, params); updErr != nil {
				common.SysError("marketplace task update failed: " + updErr.Error())
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	common.SysError("marketplace task row not found for " + publicTaskID)
}

// DoResponse writes the synthetic queued body to the client using the public
// task id, and returns the order id as the "upstream" id stored on the Task.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	orderID := c.GetString(ctxKeyOrderID)
	// Write the raw JSON body verbatim (already public-task-id shaped).
	c.Data(http.StatusOK, "application/json", body)
	return orderID, body, nil
}

// FetchTask returns the locally-tracked Task state; there is no external
// upstream to poll. The background executor is the source of truth, so this is
// only reached by the polling sweep — reading the row is safe and idempotent.
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, _ := body["task_id"].(string)
	task, exist, err := model.GetByOnlyTaskId(taskID)
	if err != nil || !exist || task == nil || len(task.Data) == 0 {
		return syntheticResponse([]byte(`{"status":"in_progress"}`)), nil
	}
	return syntheticResponse(task.Data), nil
}

// ParseTaskResult maps the stored Task data (OpenAI video shape) to a TaskInfo.
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var v dto.OpenAIVideo
	if err := common.Unmarshal(respBody, &v); err != nil {
		return &relaycommon.TaskInfo{Status: model.TaskStatusInProgress}, nil
	}
	ti := &relaycommon.TaskInfo{}
	switch v.Status {
	case dto.VideoStatusQueued:
		ti.Status = model.TaskStatusQueued
	case dto.VideoStatusInProgress:
		ti.Status = model.TaskStatusInProgress
	case dto.VideoStatusCompleted:
		ti.Status = model.TaskStatusSuccess
	case dto.VideoStatusFailed:
		ti.Status = model.TaskStatusFailure
		if v.Error != nil {
			ti.Reason = v.Error.Message
		}
	default:
		ti.Status = model.TaskStatusInProgress
	}
	return ti, nil
}

// ConvertToOpenAIVideo returns the Task's stored OpenAI-video body, ensuring the
// id is the public task id.
func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	if len(task.Data) == 0 {
		v := dto.NewOpenAIVideo()
		v.ID = task.TaskID
		return common.Marshal(v)
	}
	var v dto.OpenAIVideo
	if err := common.Unmarshal(task.Data, &v); err != nil {
		return task.Data, nil
	}
	v.ID = task.TaskID
	return common.Marshal(v)
}

// extractResultURL pulls a video/artifact URL from the script result JSON,
// checking the common keys a video script would return.
func extractResultURL(result []byte) string {
	var m map[string]any
	if err := common.Unmarshal(result, &m); err != nil {
		return ""
	}
	for _, key := range []string{"url", "video_url", "output_url", "result_url"} {
		if s, ok := m[key].(string); ok && s != "" {
			return s
		}
	}
	// Some scripts nest under "result" or "output".
	for _, key := range []string{"result", "output"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k := range []string{"url", "video_url"} {
				if s, ok := nested[k].(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// decodeResult parses the result JSON into a generic value for embedding in the
// video metadata; falls back to the raw string when not JSON.
func decodeResult(result []byte) any {
	var v any
	if err := common.Unmarshal(result, &v); err == nil {
		return v
	}
	return string(result)
}
