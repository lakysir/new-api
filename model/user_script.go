package model

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"
)

const UserScriptMaxCodeLength = 1024 * 1024

// Script review lifecycle for a draft (UserScript). Publishing an approved
// draft freezes an immutable ScriptVersion; the draft itself can keep evolving.
const (
	ScriptReviewDraft      = "draft"    // author is still editing
	ScriptReviewPending    = "pending"  // submitted, awaiting review
	ScriptReviewApproved   = "approved" // review passed, publishable
	ScriptReviewRejected   = "rejected" // review failed, back to author
	ScriptReviewPublishing = "publishing"
	ScriptReviewPublished  = "published"
)

type UserScript struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId        int    `json:"user_id" gorm:"index;not null"`
	Title         string `json:"title" gorm:"type:varchar(128);not null"`
	Description   string `json:"description" gorm:"type:text"`
	ScriptParams  string `json:"script_params" gorm:"type:text"`
	DraftCode     string `json:"draft_code,omitempty" gorm:"type:text"`
	PublishedCode string `json:"published_code,omitempty" gorm:"type:text"`
	Published     bool   `json:"published" gorm:"index"`
	PublishedAt   int64  `json:"published_at" gorm:"bigint;default:0"`
	ReviewStatus  string `json:"review_status" gorm:"type:varchar(16);index;default:draft"`
	ReviewNote    string `json:"review_note,omitempty" gorm:"type:varchar(512)"`
	LatestVersion int    `json:"latest_version" gorm:"default:0"`
	// Concurrency is the maximum number of simultaneous executions this script
	// supports on a single node (default 1). A browser node that has multiple
	// third-party tabs open simultaneously can run up to this many tasks in
	// parallel for this script. The value is snapshotted into ScriptVersion at
	// publish time so older versions keep their original concurrency semantics.
	Concurrency int `json:"concurrency" gorm:"default:1;not null"`
	// AuthorShareRatePpm: the author's cut, proposed at submit-review (ppm of the
	// provider execution price). PlatformFeeRatePpm: platform service fee, set by
	// the operator at review. Both feed the immutable pricing_template at publish.
	AuthorShareRatePpm int64 `json:"author_share_rate_ppm" gorm:"default:0"`
	PlatformFeeRatePpm int64 `json:"platform_fee_rate_ppm" gorm:"default:0"`
	// CategoryId assigns the script to a target-site category (author sets it).
	// The category's balance-probe gates whether a node may list this script.
	CategoryId            int            `json:"category_id" gorm:"default:0;index"`
	CreatedAt             int64          `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt             int64          `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
	DeletedAt             gorm.DeletedAt `json:"-" gorm:"index"`
	CodePreview           string         `json:"code_preview,omitempty" gorm:"-"`
	PreviewTruncated      bool           `json:"preview_truncated,omitempty" gorm:"-"`
	AuthorUsername        string         `json:"author_username,omitempty" gorm:"-"`
	HasUnpublishedChanges bool           `json:"has_unpublished_changes" gorm:"-"`
	PreviousTitle         string         `json:"previous_title,omitempty" gorm:"-"`
	PreviousDescription   string         `json:"previous_description,omitempty" gorm:"-"`
	PreviousScriptParams  string         `json:"previous_script_params,omitempty" gorm:"-"`
	PreviousCode          string         `json:"previous_code,omitempty" gorm:"-"`
}

func (UserScript) TableName() string {
	return "user_scripts"
}

func NormalizeUserScriptInput(title string, description string, scriptParams string, code string) (string, string, string, string, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	scriptParams = strings.TrimSpace(scriptParams)
	if title == "" {
		return "", "", "", "", errors.New("title is required")
	}
	if len([]rune(title)) > 128 {
		return "", "", "", "", errors.New("title is too long")
	}
	if len([]byte(description)) > 4096 {
		return "", "", "", "", errors.New("description is too long")
	}
	if len([]byte(scriptParams)) > 65536 {
		return "", "", "", "", errors.New("script params is too large")
	}
	if scriptParams != "" {
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(scriptParams), &decoded); err != nil || decoded == nil {
			return "", "", "", "", errors.New("script params must be a JSON object")
		}
	}
	if len([]byte(code)) > UserScriptMaxCodeLength {
		return "", "", "", "", errors.New("script code is too large")
	}
	return title, description, scriptParams, code, nil
}

func userScriptPreview(code string) (string, bool) {
	if code == "" {
		return "", false
	}
	runes := []rune(code)
	half := len(runes) / 2
	if half < 1 {
		half = 1
	}
	return string(runes[:half]), half < len(runes)
}

func (script *UserScript) FillPublishedPreview() {
	code := script.PublishedCode
	script.DraftCode = ""
	script.PublishedCode = ""
	script.CodePreview, script.PreviewTruncated = userScriptPreview(code)
}

func ListPublishedUserScripts(offset int, limit int) ([]UserScript, int64, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	publishedVersionExists := "EXISTS (SELECT 1 FROM script_versions sv WHERE sv.script_id = user_scripts.id AND sv.version = user_scripts.latest_version AND sv.review_status = ? AND sv.revoked_at = 0)"
	if err := DB.Model(&UserScript{}).Where("published = ?", true).
		Where(publishedVersionExists, ScriptVersionApproved).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var scripts []UserScript
	err := DB.Table("user_scripts").
		Select("user_scripts.id,user_scripts.user_id,user_scripts.concurrency,script_versions.title,script_versions.description,script_versions.script_params,user_scripts.published,script_versions.published_at,user_scripts.created_at,script_versions.published_at AS updated_at,script_versions.code AS published_code").
		Joins("JOIN script_versions ON script_versions.script_id = user_scripts.id AND script_versions.version = user_scripts.latest_version").
		Where("published = ?", true).
		Where(publishedVersionExists, ScriptVersionApproved).
		Order("user_scripts.updated_at desc,user_scripts.id desc").
		Offset(offset).
		Limit(limit).
		Find(&scripts).Error
	if err != nil {
		return nil, 0, err
	}
	for i := range scripts {
		code := scripts[i].PublishedCode
		scripts[i].PublishedCode = ""
		scripts[i].DraftCode = ""
		scripts[i].CodePreview, scripts[i].PreviewTruncated = userScriptPreview(code)
	}
	return scripts, total, nil
}

func GetPublishedUserScript(id int) (*UserScript, error) {
	var script UserScript
	err := DB.Table("user_scripts").
		Select("user_scripts.id,user_scripts.user_id,script_versions.title,script_versions.description,script_versions.script_params,user_scripts.published,script_versions.published_at,user_scripts.created_at,script_versions.published_at AS updated_at,script_versions.code AS published_code").
		Joins("JOIN script_versions ON script_versions.script_id = user_scripts.id AND script_versions.version = user_scripts.latest_version").
		Where("user_scripts.id = ? AND published = ?", id, true).
		Where("EXISTS (SELECT 1 FROM script_versions sv WHERE sv.script_id = user_scripts.id AND sv.version = user_scripts.latest_version AND sv.review_status = ? AND sv.revoked_at = 0)", ScriptVersionApproved).
		First(&script).Error
	if err != nil {
		return nil, err
	}
	code := script.PublishedCode
	script.PublishedCode = ""
	script.DraftCode = ""
	script.CodePreview, script.PreviewTruncated = userScriptPreview(code)
	return &script, nil
}

func GetPublishedUserScriptCode(id int) (*UserScript, error) {
	var script UserScript
	err := DB.Table("user_scripts").
		Select("user_scripts.id,user_scripts.user_id,script_versions.title,script_versions.description,script_versions.script_params,user_scripts.published,script_versions.published_at,user_scripts.created_at,script_versions.published_at AS updated_at,script_versions.code AS published_code").
		Joins("JOIN script_versions ON script_versions.script_id = user_scripts.id AND script_versions.version = user_scripts.latest_version").
		Where("user_scripts.id = ? AND published = ?", id, true).
		Where("EXISTS (SELECT 1 FROM script_versions sv WHERE sv.script_id = user_scripts.id AND sv.version = user_scripts.latest_version AND sv.review_status = ? AND sv.revoked_at = 0)", ScriptVersionApproved).
		First(&script).Error
	if err != nil {
		return nil, err
	}
	return &script, nil
}

func ListUserScripts(userId int) ([]UserScript, error) {
	var scripts []UserScript
	err := DB.Select("id,user_id,title,description,script_params,draft_code,published,published_at,review_status,review_note,latest_version,concurrency,created_at,updated_at").
		Where("user_id = ?", userId).
		Order("user_scripts.updated_at desc,user_scripts.id desc").
		Find(&scripts).Error
	if err != nil {
		return nil, err
	}
	return scripts, nil
}

// ListScriptsByReviewStatus returns scripts in a given review status. The draft
// code is included (the reviewer needs it); the last published version's
// title/description/params/code are attached via fillPreviousPublishedVersions
// so the review console can diff the pending draft against what is currently
// live. Used by the operator review console (admin).
func ListScriptsByReviewStatus(status string) ([]UserScript, error) {
	var scripts []UserScript
	err := DB.Table("user_scripts").
		Select("user_scripts.id,user_scripts.user_id,user_scripts.title,user_scripts.description,user_scripts.script_params,user_scripts.draft_code,user_scripts.review_status,user_scripts.review_note,user_scripts.latest_version,user_scripts.concurrency,user_scripts.author_share_rate_ppm,user_scripts.platform_fee_rate_ppm,user_scripts.category_id,user_scripts.published,user_scripts.published_at,user_scripts.created_at,user_scripts.updated_at").
		Where("user_scripts.review_status = ?", status).
		Order("user_scripts.updated_at desc,user_scripts.id desc").
		Find(&scripts).Error
	if err != nil {
		return nil, err
	}
	// Previous* fields are gorm:"-" (not backed by a user_scripts column), so a
	// JOIN alias cannot populate them — GORM drops unmapped columns during scan.
	// Fill them explicitly from the last published version, like AuthorUsername.
	fillPreviousPublishedVersions(scripts)
	for i := range scripts {
		scripts[i].AuthorUsername, _ = GetUsernameById(scripts[i].UserId, true)
	}
	return scripts, nil
}

// fillPreviousPublishedVersions loads, for each script that has a published
// version, the ScriptVersion that user_scripts.latest_version points at and
// copies its title/description/params/code into the Previous* fields. Scripts
// with no published version (latest_version == 0) are left blank, which the
// review console renders as "Initial version". Best-effort: a lookup error
// leaves the Previous* fields empty rather than failing the whole listing.
func fillPreviousPublishedVersions(scripts []UserScript) {
	scriptIds := make([]int, 0, len(scripts))
	for i := range scripts {
		if scripts[i].LatestVersion > 0 {
			scriptIds = append(scriptIds, scripts[i].Id)
		}
	}
	if len(scriptIds) == 0 {
		return
	}
	var versions []ScriptVersion
	if err := DB.Where("script_id IN ?", scriptIds).Find(&versions).Error; err != nil {
		return
	}
	type versionKey struct{ scriptId, version int }
	byKey := make(map[versionKey]*ScriptVersion, len(versions))
	for i := range versions {
		byKey[versionKey{versions[i].ScriptId, versions[i].Version}] = &versions[i]
	}
	for i := range scripts {
		v := byKey[versionKey{scripts[i].Id, scripts[i].LatestVersion}]
		if v == nil {
			continue
		}
		scripts[i].PreviousTitle = v.Title
		scripts[i].PreviousDescription = v.Description
		scripts[i].PreviousScriptParams = v.ScriptParams
		scripts[i].PreviousCode = v.Code
	}
}

func GetUserScriptById(id int, userId int) (*UserScript, error) {
	var script UserScript
	err := DB.Where("id = ? AND user_id = ?", id, userId).First(&script).Error
	if err != nil {
		return nil, err
	}
	return &script, nil
}

func UpsertUserScriptDraft(userId int, id int, title string, description string, scriptParams string, code string, concurrency int) (*UserScript, error) {
	title, description, scriptParams, code, err := NormalizeUserScriptInput(title, description, scriptParams, code)
	if err != nil {
		return nil, err
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if id > 0 {
		script, err := GetUserScriptById(id, userId)
		if err != nil {
			return nil, err
		}
		draftChanged := script.Title != title || script.Description != description ||
			script.ScriptParams != scriptParams || script.DraftCode != code ||
			script.Concurrency != concurrency
		script.Title = title
		script.Description = description
		script.ScriptParams = scriptParams
		script.DraftCode = code
		script.Concurrency = concurrency
		if draftChanged {
			script.ReviewStatus = ScriptReviewDraft
			script.ReviewNote = ""
		}
		if err := DB.Save(script).Error; err != nil {
			return nil, err
		}
		return script, nil
	}
	script := &UserScript{
		UserId:       userId,
		Title:        title,
		Description:  description,
		ScriptParams: scriptParams,
		DraftCode:    code,
		Concurrency:  concurrency,
	}
	if err := DB.Create(script).Error; err != nil {
		return nil, err
	}
	return script, nil
}
