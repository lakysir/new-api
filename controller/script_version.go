package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/scriptregistry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// platformSigner builds the Ed25519 signer from configured platform keys. It
// returns (nil, nil) when unconfigured so dev publishing can proceed unsigned;
// production gates this via the upline checklist.
func platformSigner() (*scriptregistry.Signer, error) {
	if common.ScriptSigningKeySeed == "" {
		return nil, nil
	}
	return scriptregistry.NewSigner(common.ScriptSigningKeyId, common.ScriptSigningKeySeed)
}

func draftMatchesLatestVersion(script *model.UserScript) (bool, error) {
	if script.LatestVersion <= 0 {
		return false, nil
	}
	version, err := model.GetScriptVersion(script.Id, script.LatestVersion)
	if err != nil {
		return false, err
	}
	normalized, codeSha256, _, err := scriptregistry.ValidatePublishable(script.DraftCode)
	if err != nil {
		return false, err
	}
	return version.Title == script.Title && version.Description == script.Description &&
		version.ScriptParams == script.ScriptParams && version.Code == normalized &&
		version.CodeSha256 == codeSha256, nil
}

// SubmitScriptForReview moves an author's draft into the pending-review state.
// The draft must have code and must expose the runGeneratedTest entry.
func SubmitScriptForReview(c *gin.Context) {
	id, ok := parseScriptId(c)
	if !ok {
		return
	}
	// The author proposes their share (ppm), target-site category, base price,
	// pricing rules and min interval when submitting for review.
	var body struct {
		AuthorShareRatePpm int64  `json:"author_share_rate_ppm"`
		CategoryId         int    `json:"category_id"`
		BasePriceMicros    int64  `json:"base_price_micros"`
		PricingRules       string `json:"pricing_rules"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.AuthorShareRatePpm < 0 || body.AuthorShareRatePpm > 50_000 {
		common.ApiErrorMsg(c, "author_share_rate_ppm must be within [0, 50000]")
		return
	}
	if body.BasePriceMicros < 0 {
		common.ApiErrorMsg(c, "base_price_micros must be non-negative")
		return
	}
	if body.CategoryId > 0 {
		if _, err := model.GetScriptCategory(body.CategoryId); err != nil {
			common.ApiErrorMsg(c, "category not found")
			return
		}
	}
	script, err := model.GetUserScriptById(id, c.GetInt("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "script not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	if script.ReviewStatus == model.ScriptReviewPending {
		common.ApiErrorMsg(c, "script is already pending review")
		return
	}
	if script.ReviewStatus == model.ScriptReviewApproved || script.ReviewStatus == model.ScriptReviewPublishing {
		common.ApiErrorMsg(c, "script is already approved for publishing")
		return
	}
	unchanged, err := draftMatchesLatestVersion(script)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if unchanged {
		common.ApiErrorMsg(c, "draft has no changes since the latest published version")
		return
	}
	if _, _, _, err := scriptregistry.ValidatePublishable(script.DraftCode); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	script.ReviewStatus = model.ScriptReviewPending
	script.ReviewNote = ""
	script.AuthorShareRatePpm = body.AuthorShareRatePpm
	script.CategoryId = body.CategoryId
	if body.BasePriceMicros > 0 {
		script.BasePriceMicros = body.BasePriceMicros
	}
	if body.PricingRules != "" {
		script.PricingRules = body.PricingRules
	}
	if err := model.DB.Model(script).Updates(map[string]any{
		"review_status":        script.ReviewStatus,
		"review_note":          "",
		"author_share_rate_ppm": body.AuthorShareRatePpm,
		"category_id":          body.CategoryId,
		"base_price_micros":    script.BasePriceMicros,
		"pricing_rules":        script.PricingRules,
	}).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, script)
}

type scriptReviewDecisionRequest struct {
	Approve bool   `json:"approve"`
	Note    string `json:"note"`
	// PlatformFeeRatePpm is the platform service fee the operator sets while
	// approving (parts-per-million of the provider execution price).
	PlatformFeeRatePpm int64 `json:"platform_fee_rate_ppm"`
}

// ListPendingScripts returns scripts awaiting review, for the operator console.
func ListPendingScripts(c *gin.Context) {
	scripts, err := model.ListScriptsByReviewStatus(model.ScriptReviewPending)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, scripts)
}

func ListPublishedScriptVersions(c *gin.Context) {
	versions, err := model.ListPublishedScriptVersions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, versions)
}

// GetPlatformScriptKey returns the platform's Ed25519 script-signing public key
// and key id. The public key is not a secret — plugins fetch it to verify
// market-script signatures (S-005), so this endpoint needs no auth. Returns an
// empty key in dev mode (unsigned publishing).
func GetPlatformScriptKey(c *gin.Context) {
	signer, err := platformSigner()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if signer == nil {
		common.ApiSuccess(c, gin.H{"key_id": "", "public_key": "", "signing_enabled": false})
		return
	}
	common.ApiSuccess(c, gin.H{
		"key_id":          signer.KeyID(),
		"public_key":      signer.PublicKeyBase64(),
		"signing_enabled": true,
	})
}

type generateSigningKeyRequest struct {
	KeyId string `json:"key_id"`
}

// GenerateScriptSigningKey (admin) creates and persists a fresh Ed25519 platform
// signing key. This ENABLES signing when it was previously unconfigured, and
// ROTATES it otherwise — rotation invalidates every existing manifest
// signature, so already-published versions must be re-published to run again.
// The private seed is stored as a masked option and never returned.
func GenerateScriptSigningKey(c *gin.Context) {
	var req generateSigningKeyRequest
	_ = c.ShouldBindJSON(&req)
	keyId := strings.TrimSpace(req.KeyId)
	if keyId == "" {
		keyId = "script-key-" + strconv.FormatInt(common.GetTimestamp(), 10)
	}
	seedB64, publicKeyB64, err := scriptregistry.GenerateSeed()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Persist both atomically; updateOptionMap also refreshes the live package
	// vars so signing works immediately without a restart.
	if err := model.UpdateOptionsBulk(map[string]string{
		"ScriptSigningKeyId":   keyId,
		"ScriptSigningKeySeed": seedB64,
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"key_id":          keyId,
		"public_key":      publicKeyB64,
		"signing_enabled": true,
	})
}

// ReviewScriptDecision is the operator endpoint to approve or reject a pending
// draft. Approval only marks the draft publishable; it does not freeze a
// version (publishing does that).
func ReviewScriptDecision(c *gin.Context) {
	id, ok := parseScriptId(c)
	if !ok {
		return
	}
	var req scriptReviewDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	var script model.UserScript
	if err := model.DB.Where("id = ?", id).First(&script).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "script not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	if script.ReviewStatus != model.ScriptReviewPending {
		common.ApiErrorMsg(c, "script is not pending review")
		return
	}
	newStatus := model.ScriptReviewApproved
	if !req.Approve {
		if strings.TrimSpace(req.Note) == "" {
			common.ApiErrorMsg(c, "rejection reason is required")
			return
		}
		newStatus = model.ScriptReviewRejected
	}
	if req.PlatformFeeRatePpm < 0 || req.PlatformFeeRatePpm > 1_000_000 {
		common.ApiErrorMsg(c, "platform_fee_rate_ppm must be within [0, 1000000]")
		return
	}
	updates := map[string]any{"review_status": newStatus, "review_note": req.Note}
	// The operator's platform fee is recorded on approval (feeds the pricing
	// template at publish, alongside the author's proposed share).
	if req.Approve {
		updates["platform_fee_rate_ppm"] = req.PlatformFeeRatePpm
	}
	if err := model.DB.Model(&script).Updates(updates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	script.ReviewStatus = newStatus
	script.ReviewNote = req.Note
	if req.Approve {
		script.PlatformFeeRatePpm = req.PlatformFeeRatePpm
	}
	common.ApiSuccess(c, script)
}

// PublishScriptVersion freezes an approved draft into a new immutable, signed
// ScriptVersion. Republishing after edits creates a new version and never
// overwrites prior code (architecture §5.3).
func PublishScriptVersion(c *gin.Context) {
	id, ok := parseScriptId(c)
	if !ok {
		return
	}
	// Optional pricing_template_id binds this version to an immutable pricing
	// template; when omitted the order layer falls back to a zero-fee default.
	var body struct {
		PricingTemplateId int `json:"pricing_template_id"`
	}
	_ = c.ShouldBindJSON(&body)
	script, err := model.GetUserScriptById(id, c.GetInt("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "script not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	if script.ReviewStatus != model.ScriptReviewApproved {
		common.ApiErrorMsg(c, "script must be approved before publishing")
		return
	}
	// Signing is mandatory (the plugin execution gate requires a valid
	// signature). Fail fast before creating a pricing template / version row so a
	// missing key never leaves orphaned unsigned state.
	if signer, signerErr := platformSigner(); signerErr != nil {
		common.ApiError(c, signerErr)
		return
	} else if signer == nil {
		common.ApiErrorMsg(c, ErrSigningKeyNotConfigured.Error())
		return
	}
	// Determine the pricing template. Prefer an explicit id; otherwise create an
	// immutable template from the reviewed author share + platform fee so the
	// version carries the fees agreed during review.
	templateId := body.PricingTemplateId
	if templateId > 0 {
		if _, err := model.GetPricingTemplate(templateId); err != nil {
			common.ApiErrorMsg(c, "pricing template not found")
			return
		}
	} else {
		tpl := &model.PricingTemplate{
			Currency:           "USD",
			ProviderPriceMode:  "per_task",
			AuthorShareRatePPM: script.AuthorShareRatePpm,
			PlatformFeeRatePPM: script.PlatformFeeRatePpm,
			FailurePolicy:      "full_refund",
			RuleVersion:        "v" + strconv.FormatInt(common.GetTimestamp(), 10),
		}
		if err := model.CreatePricingTemplate(tpl); err != nil {
			common.ApiError(c, err)
			return
		}
		templateId = tpl.Id
	}
	normalized, codeSha256, _, err := scriptregistry.ValidatePublishable(script.DraftCode)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	allowedOrigins := "[]"
	if script.CategoryId > 0 {
		if category, categoryErr := model.GetScriptCategory(script.CategoryId); categoryErr == nil {
			if origin := siteOrigin(category.Site); origin != "" {
				if encoded, marshalErr := common.Marshal([]string{origin}); marshalErr == nil {
					allowedOrigins = string(encoded)
				}
			}
		}
	}
	version := &model.ScriptVersion{
		ScriptId:          script.Id,
		AuthorId:          script.UserId,
		Title:             script.Title,
		Description:       script.Description,
		ScriptParams:      script.ScriptParams,
		AllowedOrigins:    allowedOrigins,
		TimeoutSeconds:    180,
		CategoryId:        script.CategoryId,
		PricingTemplateId: templateId,
		// Snapshot execution semantics so they are immutable per version.
		Concurrency: func() int {
			if script.Concurrency < 1 {
				return 1
			}
			return script.Concurrency
		}(),
		MinIntervalSeconds: func() int {
			if script.MinIntervalSeconds < 0 {
				return 0
			}
			return script.MinIntervalSeconds
		}(),
		BasePriceMicros: script.BasePriceMicros,
		PricingRules:    script.PricingRules,
		Code:            normalized,
		CodeSha256:      codeSha256,
		ReviewStatus:    model.ScriptVersionApproved,
		PublishedAt:     common.GetTimestamp(),
	}
	claim := model.DB.Model(&model.UserScript{}).
		Where("id = ? AND user_id = ? AND review_status = ?", script.Id, script.UserId, model.ScriptReviewApproved).
		Update("review_status", model.ScriptReviewPublishing)
	if claim.Error != nil {
		common.ApiError(c, claim.Error)
		return
	}
	if claim.RowsAffected != 1 {
		common.ApiErrorMsg(c, "script is already being published or is no longer approved")
		return
	}
	restoreApproval := func() {
		_ = model.DB.Model(&model.UserScript{}).
			Where("id = ? AND review_status = ?", script.Id, model.ScriptReviewPublishing).
			Update("review_status", model.ScriptReviewApproved).Error
	}

	// Assign the next version number first so it is part of the signed manifest.
	if err := model.CreateScriptVersion(version); err != nil {
		restoreApproval()
		common.ApiError(c, err)
		return
	}

	if err := signPublishedVersion(version); err != nil {
		restoreApproval()
		common.ApiError(c, err)
		return
	}

	// Keep the legacy published-code path working during migration.
	script.PublishedCode = normalized
	script.Published = true
	script.PublishedAt = version.PublishedAt
	_ = model.DB.Model(script).Updates(map[string]any{
		"published_code": normalized, "published": true, "published_at": version.PublishedAt,
		"review_status": model.ScriptReviewPublished, "review_note": "",
	}).Error

	common.ApiSuccess(c, gin.H{
		"script_id":   version.ScriptId,
		"version":     version.Version,
		"code_sha256": version.CodeSha256,
		"signed":      version.Signature != "",
	})
}

// siteOrigin converts a category site (full URL or bare hostname) to the
// browser origin stored in the signed script manifest.
func siteOrigin(site string) string {
	raw := strings.TrimSpace(site)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// ErrSigningKeyNotConfigured is returned when a publish is attempted while the
// platform has no signing key. Execution requires a valid signature, so
// publishing unsigned versions is refused rather than silently producing
// versions that can never run.
var ErrSigningKeyNotConfigured = errors.New("platform script signing key is not configured; an administrator must generate it in the script review console before publishing")

// signPublishedVersion signs the manifest for a freshly created version and
// persists the signature. Signing is mandatory: without a configured key the
// version could never pass the plugin execution gate, so we return an error
// instead of storing an unsigned, unusable version.
func signPublishedVersion(version *model.ScriptVersion) error {
	signer, err := platformSigner()
	if err != nil {
		return err
	}
	if signer == nil {
		return ErrSigningKeyNotConfigured
	}
	manifest := scriptregistry.Manifest{
		ScriptID:       strconv.Itoa(version.ScriptId),
		Version:        strconv.Itoa(version.Version),
		Title:          version.Title,
		TaskType:       version.TaskType,
		AllowedOrigins: decodeOrigins(version.AllowedOrigins),
		ParamsSchema:   version.ScriptParams,
		ResultSchema:   version.ResultSchema,
		TimeoutSeconds: version.TimeoutSeconds,
		CodeSha256:     version.CodeSha256,
		ReviewStatus:   version.ReviewStatus,
		PublishedAt:    version.PublishedAt,
	}
	if _, err := signer.Sign(&manifest); err != nil {
		return err
	}
	return model.DB.Model(&model.ScriptVersion{}).Where("id = ?", version.Id).Updates(map[string]any{
		"signature":        manifest.Signature,
		"signature_key_id": manifest.SignatureKeyID,
	}).Error
}

func decodeOrigins(raw string) []string {
	out := []string{}
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

type revokeVersionRequest struct {
	Reason   string `json:"reason"`
	Severity string `json:"severity"`
}

// RevokeScriptVersion is the operator endpoint to revoke a published version.
// It stops new tasks and never mutates the frozen code/hash/signature.
func RevokeScriptVersion(c *gin.Context) {
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
	var req revokeVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		common.ApiErrorMsg(c, "revoke reason is required")
		return
	}
	if req.Severity == "" {
		req.Severity = "normal"
	}
	if err := model.RevokeScriptVersion(scriptId, version, req.Reason, req.Severity); err != nil {
		if errors.Is(err, model.ErrScriptVersionNotFound) {
			common.ApiErrorMsg(c, "script version not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	// Cascade: suspend any node capabilities bound to the revoked version so no
	// new tasks are dispatched to them (PRD N-007).
	suspended, _ := model.SuspendCapabilitiesByScriptVersion(scriptId, version)
	common.ApiSuccess(c, gin.H{"suspended_capabilities": suspended})
}

func DeleteScriptVersion(c *gin.Context) {
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
	if err := model.DeleteHistoricalScriptVersion(scriptId, version); err != nil {
		switch {
		case errors.Is(err, model.ErrScriptVersionNotFound):
			common.ApiErrorMsg(c, "script version not found")
		case errors.Is(err, model.ErrLatestScriptVersion):
			common.ApiErrorMsg(c, err.Error())
		default:
			common.ApiError(c, err)
		}
		return
	}
	common.ApiSuccess(c, gin.H{"deleted": true})
}

func UpdateScriptVersionPricing(c *gin.Context) {
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
	var req struct {
		AuthorShareRatePPM int64 `json:"author_share_rate_ppm"`
		PlatformFeeRatePPM int64 `json:"platform_fee_rate_ppm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.AuthorShareRatePPM < 0 || req.AuthorShareRatePPM > 1_000_000 || req.PlatformFeeRatePPM < 0 || req.PlatformFeeRatePPM > 1_000_000 {
		common.ApiErrorMsg(c, "invalid pricing values")
		return
	}
	updated, err := model.UpdateScriptVersionPricing(scriptId, version, req.AuthorShareRatePPM, req.PlatformFeeRatePPM)
	if err != nil {
		if errors.Is(err, model.ErrScriptVersionNotFound) {
			common.ApiErrorMsg(c, "script version not found")
		} else {
			common.ApiError(c, err)
		}
		return
	}
	common.ApiSuccess(c, updated)
}

// ListScriptVersions returns a script's version history (no code bodies).
func ListScriptVersions(c *gin.Context) {
	scriptId, err := strconv.Atoi(c.Param("id"))
	if err != nil || scriptId <= 0 {
		common.ApiErrorMsg(c, "invalid script id")
		return
	}
	versions, err := model.ListScriptVersions(scriptId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, versions)
}

// ListExecutableScriptVersions returns published versions available for node
// capability listing.
func ListExecutableScriptVersions(c *gin.Context) {
	scriptId, err := strconv.Atoi(c.Param("id"))
	if err != nil || scriptId <= 0 {
		common.ApiErrorMsg(c, "invalid script id")
		return
	}
	versions, err := model.ListExecutableScriptVersions(scriptId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, versions)
}

// GetFixedScriptVersion returns the manifest + code for a fixed, non-revoked,
// approved version. This is the interface order execution must use (S-005):
// plugins verify code_sha256 and signature before running.
func GetFixedScriptVersion(c *gin.Context) {
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
	v, err := model.GetExecutableScriptVersion(scriptId, version)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrScriptVersionRevoked):
			c.JSON(http.StatusGone, gin.H{"success": false, "message": "script version revoked", "error_code": "SCRIPT_REVOKED"})
		case errors.Is(err, model.ErrScriptVersionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "script version not found", "error_code": "SCRIPT_NOT_FOUND"})
		default:
			common.ApiError(c, err)
		}
		return
	}
	common.ApiSuccess(c, gin.H{
		"manifest": gin.H{
			"scriptId":       strconv.Itoa(v.ScriptId),
			"version":        strconv.Itoa(v.Version),
			"title":          v.Title,
			"taskType":       v.TaskType,
			"allowedOrigins": decodeOrigins(v.AllowedOrigins),
			"paramsSchema":   v.ScriptParams,
			"resultSchema":   v.ResultSchema,
			"timeoutSeconds": v.TimeoutSeconds,
			"codeSha256":     v.CodeSha256,
			"reviewStatus":   v.ReviewStatus,
			"publishedAt":    v.PublishedAt,
			"signatureKeyId": v.SignatureKeyId,
			"signature":      v.Signature,
		},
		"code": v.Code,
	})
}
