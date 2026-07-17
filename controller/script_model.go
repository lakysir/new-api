package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// script_model.go exposes the operator actions that publish a marketplace
// script version as a callable new-api model (and the inverse). Publishing
// creates a ScriptModelBinding and adds the model name to the shared
// marketplace channel so it becomes routable (see model.PublishScriptModel).
// Orders for the model are later funded from the publishing operator's
// marketplace available balance by the bridge adaptor.

type publishScriptModelRequest struct {
	ModelName         string `json:"model_name"`
	ConsumeMultiplier int64  `json:"consume_multiplier"`
	ParamTemplate     string `json:"param_template"`
}

// PublishScriptAsModel binds a published, non-balance-probe script version to a
// unique model name and makes it routable. Admin-only (route is under
// scriptAdminRoute). The publishing operator (c.id) funds future orders.
func PublishScriptAsModel(c *gin.Context) {
	scriptId, err := strconv.Atoi(c.Param("id"))
	if err != nil || scriptId <= 0 {
		common.ApiErrorMsg(c, "invalid script id")
		return
	}
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil || version <= 0 {
		common.ApiErrorMsg(c, "invalid version")
		return
	}
	var req publishScriptModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	modelName := strings.TrimSpace(req.ModelName)
	if modelName == "" {
		common.ApiErrorMsg(c, "model_name is required")
		return
	}
	if req.ParamTemplate != "" && !isValidJSONObject(req.ParamTemplate) {
		common.ApiErrorMsg(c, "param_template must be a JSON object")
		return
	}

	// The version must be executable (approved + not revoked).
	if _, err := model.GetExecutableScriptVersion(scriptId, version); err != nil {
		common.ApiErrorMsg(c, "script version is not executable")
		return
	}
	// Balance-probe scripts read site balance without generating; never publish.
	isProbe, err := model.IsBalanceProbeScript(scriptId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if isProbe {
		common.ApiErrorMsg(c, "balance-probe scripts cannot be published as models")
		return
	}
	// The model name must not collide with an existing routable model on any
	// channel, otherwise routing between the two would be ambiguous.
	if modelInUseByOtherChannel(modelName) {
		common.ApiErrorMsg(c, "model name is already served by another channel")
		return
	}

	binding := &model.ScriptModelBinding{
		ModelName:         modelName,
		ScriptId:          scriptId,
		Version:           version,
		PublisherUserId:   c.GetInt("id"),
		ConsumeMultiplier: req.ConsumeMultiplier,
		ParamTemplate:     req.ParamTemplate,
		Enabled:           true,
	}
	if err := model.PublishScriptModel(binding); err != nil {
		if errors.Is(err, model.ErrModelNameTaken) {
			common.ApiErrorMsg(c, "model name is already in use")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, binding)
}

// UnpublishScriptModel removes a model binding and refreshes the channel.
func UnpublishScriptModel(c *gin.Context) {
	modelName := strings.TrimSpace(c.Param("model_name"))
	if modelName == "" {
		common.ApiErrorMsg(c, "model_name is required")
		return
	}
	if err := model.UnpublishScriptModel(modelName); err != nil {
		if errors.Is(err, model.ErrModelBindingNotFound) {
			common.ApiErrorMsg(c, "model binding not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ListScriptModelBindings returns all marketplace model bindings so the console
// can show which published scripts are listed as models.
func ListScriptModelBindings(c *gin.Context) {
	bindings, err := model.ListScriptModelBindings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, bindings)
}

// modelInUseByOtherChannel reports whether any enabled channel already serves
// the model name. A binding uniqueness check covers marketplace-vs-marketplace;
// this covers marketplace-vs-regular channels so routing is never ambiguous.
func modelInUseByOtherChannel(modelName string) bool {
	for _, name := range model.GetEnabledModels() {
		if name == modelName {
			return true
		}
	}
	return false
}

// isValidJSONObject reports whether s parses as a JSON object.
func isValidJSONObject(s string) bool {
	var m map[string]any
	return common.Unmarshal([]byte(s), &m) == nil
}
