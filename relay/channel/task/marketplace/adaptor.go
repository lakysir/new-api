// Package marketplace is the in-process task adaptor that bridges new-api's
// async task relay (e.g. OpenAI /v1/videos) to the AiToken P2P marketplace.
// Instead of calling an upstream HTTP API, DoRequest creates a marketplace
// order funded by the publishing operator, dispatches it to a provider node,
// and drives the E2EE data plane in-process to send the script config and
// receive the encrypted result — all without exposing plaintext to the relay.
// The result's artifact URL is written back to the Task so a later
// GET /v1/videos/{id} returns it in OpenAI shape.
package marketplace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const (
	// ctxKeyConfig / ctxKeyBinding stash values between ValidateRequestAndSetAction,
	// DoRequest and DoResponse within one relay attempt.
	ctxKeyConfig  = "marketplace_config"
	ctxKeyBinding = "marketplace_binding"
	ctxKeyOrderID = "marketplace_order_id"
)

// TaskAdaptor implements channel.TaskAdaptor and channel.OpenAIVideoConverter
// for the marketplace bridge.
type TaskAdaptor struct {
	taskcommon.BaseBilling
	modelName string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.modelName = info.OriginModelName
	if a.modelName == "" {
		a.modelName = info.UpstreamModelName
	}
}

// ValidateRequestAndSetAction parses the OpenAI-style request body and stores
// the derived script config (OpenAI fields merged over the binding template)
// on the context for DoRequest.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	modelName := a.modelName
	if modelName == "" {
		modelName = info.OriginModelName
	}
	binding, err := model.GetBindingByModelName(modelName)
	if err != nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model %q is not a marketplace model", modelName), "invalid_model", http.StatusBadRequest)
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusBadRequest)
	}
	body, err := storage.Bytes()
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusBadRequest)
	}

	config, taskErr := buildScriptConfig(binding, body)
	if taskErr != nil {
		return taskErr
	}
	c.Set(ctxKeyConfig, config)
	c.Set(ctxKeyBinding, binding)
	return nil
}

// EstimateBilling maps the request's video seconds to a "seconds" ratio so the
// new-api-side pre-charge scales with duration, mirroring sora. The marketplace
// order is priced separately (from the provider offer) and funded by the
// operator; this only affects the calling user's new-api quota.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	config, ok := c.Get(ctxKeyConfig)
	if !ok {
		return nil
	}
	seconds := extractSeconds(config)
	if seconds <= 0 {
		return nil
	}
	return map[string]float64{"seconds": float64(seconds)}
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// No upstream URL; execution is in-process over the E2EE relay.
	return "", nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	// The config was parsed in ValidateRequestAndSetAction; nothing to build.
	return nil, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	names, err := model.ListEnabledBindingModelNames()
	if err != nil {
		return []string{}
	}
	return names
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// buildScriptConfig merges the incoming OpenAI-style request fields over the
// binding's param template (template values are defaults; request fields win),
// producing the JSON config sent to the script over the E2EE relay.
func buildScriptConfig(binding *model.ScriptModelBinding, requestBody []byte) (map[string]any, *dto.TaskError) {
	config := map[string]any{}
	if binding.ParamTemplate != "" {
		if err := common.Unmarshal([]byte(binding.ParamTemplate), &config); err != nil {
			return nil, service.TaskErrorWrapperLocal(err, "invalid_param_template", http.StatusInternalServerError)
		}
	}
	if len(requestBody) > 0 {
		var reqFields map[string]any
		if err := common.Unmarshal(requestBody, &reqFields); err == nil {
			for k, v := range reqFields {
				// Drop the routing-only "model" field from the script config.
				if k == "model" {
					continue
				}
				config[k] = v
			}
		}
	}
	return config, nil
}

// extractSeconds reads a numeric "seconds" (or "duration") from the config for
// billing/consume-multiplier purposes. Returns 0 when absent/unparseable.
func extractSeconds(config any) int {
	m, ok := config.(map[string]any)
	if !ok {
		return 0
	}
	for _, key := range []string{"seconds", "duration"} {
		switch v := m[key].(type) {
		case float64:
			return int(v)
		case int:
			return v
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
	}
	return 0
}

// inputHash mirrors the browser client: sha256 of the config JSON, prefixed
// "sha256:". Only the hash crosses the control plane; the plaintext travels the
// E2EE relay.
func inputHash(config map[string]any) string {
	b, _ := common.Marshal(config)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// resultHash hashes the raw result bytes received over the relay so the client
// receipt matches the provider's receipt for reconciliation.
func resultHash(result []byte) string {
	sum := sha256.Sum256(result)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// syntheticResponse builds an in-memory HTTP 200 response so RelayTaskSubmit's
// status check passes without an upstream call.
func syntheticResponse(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}

// ensure interface compliance at compile time.
var _ channel.TaskAdaptor = (*TaskAdaptor)(nil)
var _ channel.OpenAIVideoConverter = (*TaskAdaptor)(nil)
