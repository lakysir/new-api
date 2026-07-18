package model

import (
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"gorm.io/gorm"
)

// MarketplaceChannelName is the fixed name of the shared, auto-provisioned
// channel that fronts all marketplace-bridged models. All published script
// models live on this single channel (type ChannelTypeMarketplace); the bridge
// adaptor resolves the actual script per model name from the binding table.
const MarketplaceChannelName = "aitoken-marketplace"

// ScriptModelBinding maps a marketplace script version to a new-api model name
// so the script can be invoked through the standard relay (e.g. /v1/videos).
// When an operator "publishes" a published, audited script version as a model,
// a binding row is created and the model name is added to the shared
// marketplace channel's model list (which rebuilds the abilities index so the
// model becomes routable and appears in the model square).
//
// The marketplace bridge adaptor executes the bound script over the E2EE data
// plane on behalf of PublisherUserId — orders are funded from that operator's
// marketplace available balance, never the calling user's. Only metadata and
// the input hash cross the control plane; the plaintext travels the E2EE relay.
type ScriptModelBinding struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ModelName string `json:"model_name" gorm:"type:varchar(191);uniqueIndex;not null"`
	ScriptId  int    `json:"script_id" gorm:"index;not null"`
	Version   int    `json:"version" gorm:"not null"`
	// PublisherUserId is the operator who published this model; marketplace
	// orders for it are funded from this user's available balance.
	PublisherUserId int `json:"publisher_user_id" gorm:"index;not null"`
	// ConsumeMultiplier is the default units-of-work coefficient applied when the
	// request does not carry an explicit value (e.g. video seconds). Min 1.
	ConsumeMultiplier int64 `json:"consume_multiplier" gorm:"default:1"`
	// ParamTemplate is a JSON object merged under the incoming OpenAI-style
	// request fields to build the script config sent over the E2EE relay. Empty
	// means the request body is forwarded to the script verbatim.
	ParamTemplate string `json:"param_template" gorm:"type:text"`
	Enabled       bool   `json:"enabled" gorm:"default:true"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ScriptModelBinding) TableName() string { return "script_model_bindings" }

var (
	// ErrModelBindingNotFound is returned when no binding matches a lookup.
	ErrModelBindingNotFound = errors.New("script model binding not found")
	// ErrModelNameTaken is returned when the model name is already bound.
	ErrModelNameTaken = errors.New("model name is already in use")
)

// CreateScriptModelBinding inserts a binding, rejecting a duplicate model name.
func CreateScriptModelBinding(b *ScriptModelBinding) error {
	b.ModelName = strings.TrimSpace(b.ModelName)
	if b.ModelName == "" {
		return errors.New("model name is required")
	}
	if b.ConsumeMultiplier < 1 {
		b.ConsumeMultiplier = 1
	}
	if _, err := GetBindingByModelName(b.ModelName); err == nil {
		return ErrModelNameTaken
	} else if !errors.Is(err, ErrModelBindingNotFound) {
		return err
	}
	return DB.Create(b).Error
}

// GetBindingByModelName resolves the binding for a model name (used by the
// bridge adaptor to find the script + funding operator for a relay request).
func GetBindingByModelName(modelName string) (*ScriptModelBinding, error) {
	var b ScriptModelBinding
	err := DB.Where("model_name = ?", strings.TrimSpace(modelName)).First(&b).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrModelBindingNotFound
		}
		return nil, err
	}
	return &b, nil
}

// ListScriptModelBindings returns all bindings (operator console + model list).
func ListScriptModelBindings() ([]ScriptModelBinding, error) {
	var bindings []ScriptModelBinding
	err := DB.Order("id asc").Find(&bindings).Error
	return bindings, err
}

// ListEnabledBindingModelNames returns the model names of all enabled bindings,
// used to populate the bridge adaptor's model list.
func ListEnabledBindingModelNames() ([]string, error) {
	var names []string
	err := DB.Model(&ScriptModelBinding{}).Where("enabled = ?", true).
		Order("id asc").Pluck("model_name", &names).Error
	return names, err
}

// DeleteScriptModelBindingByName removes a binding by model name. Returns
// ErrModelBindingNotFound when no row matched.
func DeleteScriptModelBindingByName(modelName string) error {
	res := DB.Where("model_name = ?", strings.TrimSpace(modelName)).Delete(&ScriptModelBinding{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrModelBindingNotFound
	}
	return nil
}

// getOrCreateMarketplaceChannel returns the shared marketplace channel,
// creating it (enabled, group "default") on first use. The channel carries no
// upstream key/base URL — the bridge adaptor executes in-process — so those are
// left empty.
func getOrCreateMarketplaceChannel() (*Channel, error) {
	var ch Channel
	err := DB.Where("type = ?", constant.ChannelTypeMarketplace).First(&ch).Error
	if err == nil {
		return &ch, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	emptyBase := ""
	priority := int64(0)
	ch = Channel{
		Type:        constant.ChannelTypeMarketplace,
		Key:         "",
		Status:      common.ChannelStatusEnabled,
		Name:        MarketplaceChannelName,
		BaseURL:     &emptyBase,
		Models:      "",
		Group:       "default",
		Priority:    &priority,
		CreatedTime: common.GetTimestamp(),
	}
	if err := ch.Insert(); err != nil {
		return nil, err
	}
	return &ch, nil
}

// syncMarketplaceChannelModels rebuilds the marketplace channel's Models list
// from the current enabled bindings and refreshes its abilities so the model
// square and the router stay in sync after a publish/unpublish.
func syncMarketplaceChannelModels() error {
	ch, err := getOrCreateMarketplaceChannel()
	if err != nil {
		return err
	}
	names, err := ListEnabledBindingModelNames()
	if err != nil {
		return err
	}
	sort.Strings(names)
	ch.Models = strings.Join(names, ",")
	// Force-write the models column with Select so an empty list (last model
	// unpublished) is persisted too — Channel.Update()/GORM Updates skips
	// zero-value fields, so an empty string would be silently dropped, leaving
	// the stale model routable. Then rebuild abilities from the fresh value.
	if err := DB.Model(ch).Select("models").Update("models", ch.Models).Error; err != nil {
		return err
	}
	return ch.UpdateAbilities(nil)
}

// PublishScriptModel creates a binding and republishes the marketplace channel
// model list atomically enough for the MVP: the binding insert is the source of
// truth; a failed channel sync leaves the binding without a routable model,
// which the caller surfaces as an error so the operator can retry.
func PublishScriptModel(b *ScriptModelBinding) error {
	if err := CreateScriptModelBinding(b); err != nil {
		return err
	}
	if err := syncMarketplaceChannelModels(); err != nil {
		// Roll back the binding so publish is all-or-nothing from the operator's
		// view (the model must be routable or the publish failed).
		_ = DeleteScriptModelBindingByName(b.ModelName)
		return err
	}
	return nil
}

// UnpublishScriptModel deletes a binding and refreshes the channel model list.
func UnpublishScriptModel(modelName string) error {
	if err := DeleteScriptModelBindingByName(modelName); err != nil {
		return err
	}
	return syncMarketplaceChannelModels()
}

// ScriptModelDoc is the public, caller-facing documentation for one bridged
// model: what it is, the parameter/result JSON Schemas the script declares, the
// operator's param template (defaults merged under the request), and the
// consume-multiplier semantics. It carries no secrets (no code, no signature).
type ScriptModelDoc struct {
	ModelName         string `json:"model_name"`
	ScriptId          int    `json:"script_id"`
	Version           int    `json:"version"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	TaskType          string `json:"task_type"`
	ScriptParams      string `json:"script_params"`  // params JSON Schema
	ResultSchema      string `json:"result_schema"`  // result JSON Schema
	ParamTemplate     string `json:"param_template"` // operator defaults (JSON)
	ConsumeMultiplier int64  `json:"consume_multiplier"`
	TimeoutSeconds    int    `json:"timeout_seconds"`
}

// GetScriptModelDoc assembles the caller-facing documentation for a bridged
// model name by joining its binding to the fixed script version. Returns
// ErrModelBindingNotFound when the name is not a marketplace model.
func GetScriptModelDoc(modelName string) (*ScriptModelDoc, error) {
	binding, err := GetBindingByModelName(modelName)
	if err != nil {
		return nil, err
	}
	// Use GetScriptVersion (not the executable variant) so docs stay visible
	// even if the version was later revoked — the model may still be listed.
	sv, err := GetScriptVersion(binding.ScriptId, binding.Version)
	if err != nil {
		return nil, err
	}
	return &ScriptModelDoc{
		ModelName:         binding.ModelName,
		ScriptId:          binding.ScriptId,
		Version:           binding.Version,
		Title:             sv.Title,
		Description:       sv.Description,
		TaskType:          sv.TaskType,
		ScriptParams:      sv.ScriptParams,
		ResultSchema:      sv.ResultSchema,
		ParamTemplate:     binding.ParamTemplate,
		ConsumeMultiplier: binding.ConsumeMultiplier,
		TimeoutSeconds:    sv.TimeoutSeconds,
	}, nil
}
