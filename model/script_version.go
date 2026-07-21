package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Review/version status shared by ScriptVersion.
const (
	ScriptVersionApproved = "approved"
	ScriptVersionRevoked  = "revoked"
)

// ScriptVersion is an immutable, signed snapshot of a script's code and
// manifest at publish time. Once created, (ScriptId, Version) never changes;
// corrections are made by publishing a new version. Revocation only sets
// RevokedAt and never mutates code/hash/signature (architecture §5.3).
type ScriptVersion struct {
	Id                int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ScriptId          int    `json:"script_id" gorm:"index:idx_script_version,unique;not null"`
	Version           int    `json:"version" gorm:"index:idx_script_version,unique;not null"`
	AuthorId          int    `json:"author_id" gorm:"index;not null"`
	Title             string `json:"title" gorm:"type:varchar(128)"`
	Description       string `json:"description" gorm:"type:text"`
	TaskType          string `json:"task_type" gorm:"type:varchar(64)"`
	CategoryId        int    `json:"category_id" gorm:"default:0;index"` // target-site category
	ScriptParams      string `json:"script_params" gorm:"type:text"`     // params JSON Schema
	ResultSchema      string `json:"result_schema,omitempty" gorm:"type:text"`
	AllowedOrigins    string `json:"allowed_origins" gorm:"type:text"` // JSON array
	TimeoutSeconds    int    `json:"timeout_seconds" gorm:"default:180"`
	PricingTemplateId int    `json:"pricing_template_id" gorm:"default:0;index"`
	// Concurrency is the max simultaneous executions this script supports per
	// node — snapshotted from UserScript.Concurrency at publish time so version
	// semantics are immutable after publish. Defaults to 1.
	Concurrency int `json:"concurrency" gorm:"default:1;not null"`
	// MinIntervalSeconds is the minimum gap in seconds between two consecutive
	// task submissions for this script on the same node. Snapshotted from the
	// author's draft at publish time. The scheduler enforces this interval;
	// 0 means no gap is required.
	MinIntervalSeconds int `json:"min_interval_seconds" gorm:"default:30;not null"`
	// BasePriceMicros is the author-set base price per execution unit in micro-USD
	// (1 USD = 1,000,000). Node providers multiply this by their PriceMultiplier.
	BasePriceMicros int64 `json:"base_price_micros" gorm:"default:0"`
	// PricingRules is a JSON array of PricingRule objects that describe how
	// individual task parameters map to price multipliers. Stored as text so it
	// is human-readable and survives schema-free evolution. Empty means flat rate.
	PricingRules string `json:"pricing_rules,omitempty" gorm:"type:text"`
	Code           string `json:"code,omitempty" gorm:"type:mediumtext"`
	CodeSha256     string `json:"code_sha256" gorm:"type:varchar(80);index"`
	SignatureKeyId string `json:"signature_key_id" gorm:"type:varchar(64)"`
	Signature      string `json:"signature" gorm:"type:varchar(256)"`
	ReviewStatus   string `json:"review_status" gorm:"type:varchar(16);index;default:approved"`
	PublishedAt    int64  `json:"published_at" gorm:"bigint;index"`
	RevokedAt      int64  `json:"revoked_at" gorm:"bigint;default:0;index"`
	RevokedReason  string `json:"revoked_reason,omitempty" gorm:"type:varchar(512)"`
	RevokeSeverity string `json:"revoke_severity,omitempty" gorm:"type:varchar(16)"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	AuthorUsername      string `json:"author_username,omitempty" gorm:"-"`
	AuthorShareRatePPM   int64  `json:"author_share_rate_ppm" gorm:"-"`
	PlatformFeeRatePPM   int64  `json:"platform_fee_rate_ppm" gorm:"-"`
}

func (ScriptVersion) TableName() string {
	return "script_versions"
}

var (
	// ErrScriptVersionNotFound is returned when a fixed version is missing.
	ErrScriptVersionNotFound = errors.New("script version not found")
	// ErrScriptVersionRevoked is returned when the version exists but is revoked.
	ErrScriptVersionRevoked = errors.New("script version is revoked")
	ErrLatestScriptVersion  = errors.New("latest script version cannot be deleted")
)

// IsRevoked reports whether the version has been revoked.
func (v *ScriptVersion) IsRevoked() bool { return v.RevokedAt > 0 }

// CreateScriptVersion inserts an immutable version, allocating the next
// sequential version number for the script. Because MAX(version)+1 is read
// without a table lock, two concurrent publishes for the same script can pick
// the same number; the unique (script_id, version) index rejects the loser,
// which we retry a bounded number of times. Cross-DB safe (no DB-specific
// locking hints), correctness guaranteed by the unique index.
func CreateScriptVersion(v *ScriptVersion) error {
	const maxRetries = 8
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		lastErr = createScriptVersionOnce(v)
		if lastErr == nil {
			return nil
		}
		if !isUniqueConstraintErr(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func createScriptVersionOnce(v *ScriptVersion) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		row := tx.Model(&ScriptVersion{}).
			Where("script_id = ?", v.ScriptId).
			Select("COALESCE(MAX(version), 0)")
		if err := row.Scan(&maxVersion).Error; err != nil {
			return err
		}
		v.Version = maxVersion + 1
		if v.PublishedAt == 0 {
			v.PublishedAt = time.Now().Unix()
		}
		if v.ReviewStatus == "" {
			v.ReviewStatus = ScriptVersionApproved
		}
		if err := tx.Create(v).Error; err != nil {
			return err
		}
		// Track the latest published version on the parent draft for listing.
		return tx.Model(&UserScript{}).
			Where("id = ?", v.ScriptId).
			Updates(map[string]any{"latest_version": v.Version, "published": true, "published_at": v.PublishedAt}).Error
	})
}

// isUniqueConstraintErr reports whether err is a unique-index violation across
// the supported drivers (Postgres 23505, MySQL 1062, SQLite UNIQUE constraint).
func isUniqueConstraintErr(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "Duplicate entry") ||
		strings.Contains(msg, "1062")
}

// GetScriptVersion returns a fixed (scriptId, version). It returns the version
// even when revoked so callers can distinguish "not found" from "revoked";
// GetExecutableScriptVersion enforces the revocation gate.
func GetScriptVersion(scriptId int, version int) (*ScriptVersion, error) {
	var v ScriptVersion
	err := DB.Where("script_id = ? AND version = ?", scriptId, version).First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScriptVersionNotFound
		}
		return nil, err
	}
	return &v, nil
}

// GetExecutableScriptVersion returns a version only if it is approved and not
// revoked, enforcing the execution-gate half of architecture §5.4.
func GetExecutableScriptVersion(scriptId int, version int) (*ScriptVersion, error) {
	v, err := GetScriptVersion(scriptId, version)
	if err != nil {
		return nil, err
	}
	if v.IsRevoked() {
		return nil, ErrScriptVersionRevoked
	}
	if v.ReviewStatus != ScriptVersionApproved {
		return nil, ErrScriptVersionNotFound
	}
	return v, nil
}

// ListScriptVersions returns a script's versions newest-first without code.
func ListScriptVersions(scriptId int) ([]ScriptVersion, error) {
	var versions []ScriptVersion
	err := DB.Where("script_id = ?", scriptId).
		Order("version desc").
		Omit("code").
		Find(&versions).Error
	return versions, err
}

// ListExecutableScriptVersions returns versions that providers may currently
// list as node capabilities, newest first.
func ListExecutableScriptVersions(scriptId int) ([]ScriptVersion, error) {
	var versions []ScriptVersion
	err := DB.Where("script_id = ? AND review_status = ? AND revoked_at = 0", scriptId, ScriptVersionApproved).
		Order("version desc").
		Omit("code").
		Find(&versions).Error
	return versions, err
}

func ListPublishedScriptVersions() ([]ScriptVersion, error) {
	var versions []ScriptVersion
	err := DB.Table("script_versions").
		Select("script_versions.id,script_versions.script_id,script_versions.version,script_versions.author_id,script_versions.title,script_versions.category_id,script_versions.concurrency,script_versions.code_sha256,script_versions.signature_key_id,script_versions.signature,script_versions.review_status,script_versions.published_at,script_versions.revoked_at,script_versions.revoked_reason,script_versions.revoke_severity,script_versions.pricing_template_id").
		Order("script_versions.published_at desc,script_versions.id desc").
		Limit(500).
		Find(&versions).Error
	if err == nil {
		fillScriptVersionAuthorUsernames(versions)
		fillScriptVersionPricing(versions)
	}
	return versions, err
}

func fillScriptVersionPricing(versions []ScriptVersion) {
	ids := make([]int, 0, len(versions))
	for i := range versions {
		if versions[i].PricingTemplateId > 0 {
			ids = append(ids, versions[i].PricingTemplateId)
		}
	}
	if len(ids) == 0 {
		return
	}
	var templates []PricingTemplate
	if err := DB.Where("id IN ?", ids).Find(&templates).Error; err != nil {
		return
	}
	byID := make(map[int]PricingTemplate, len(templates))
	for _, template := range templates {
		byID[template.Id] = template
	}
	for i := range versions {
		if template, ok := byID[versions[i].PricingTemplateId]; ok {
			versions[i].AuthorShareRatePPM = template.AuthorShareRatePPM
			versions[i].PlatformFeeRatePPM = template.PlatformFeeRatePPM
		}
	}
}

// DeleteHistoricalScriptVersion deletes a non-latest version. The latest check
// and delete share a transaction so the rule cannot be bypassed by the UI.
func DeleteHistoricalScriptVersion(scriptId, version int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var current ScriptVersion
		if err := tx.Where("script_id = ? AND version = ?", scriptId, version).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrScriptVersionNotFound
			}
			return err
		}
		var latest int
		if err := tx.Model(&ScriptVersion{}).Where("script_id = ?", scriptId).Select("COALESCE(MAX(version), 0)").Scan(&latest).Error; err != nil {
			return err
		}
		if version == latest {
			return ErrLatestScriptVersion
		}
		return tx.Delete(&current).Error
	})
}

// UpdateScriptVersionPricing creates a fresh immutable pricing template and
// rebinds the version. Existing order snapshots retain their original prices.
func UpdateScriptVersionPricing(scriptId, version int, authorRate, platformRate int64) (*ScriptVersion, error) {
	var updated ScriptVersion
	err := DB.Transaction(func(tx *gorm.DB) error {
		var current ScriptVersion
		if err := tx.Where("script_id = ? AND version = ?", scriptId, version).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrScriptVersionNotFound
			}
			return err
		}
		tpl := PricingTemplate{
			Currency: "USD", ProviderPriceMode: "per_task", AuthorShareRatePPM: authorRate,
			PlatformFeeRatePPM: platformRate, PlatformFeeMinMicros: 0,
			FailurePolicy: "full_refund", RuleVersion: "admin-" + time.Now().Format("20060102150405"),
		}
		if err := tx.Create(&tpl).Error; err != nil {
			return err
		}
		if err := tx.Model(&current).Update("pricing_template_id", tpl.Id).Error; err != nil {
			return err
		}
		current.PricingTemplateId = tpl.Id
		current.AuthorShareRatePPM, current.PlatformFeeRatePPM = authorRate, platformRate
		updated = current
		return nil
	})
	return &updated, err
}

func fillScriptVersionAuthorUsernames(versions []ScriptVersion) {
	ids := make([]int, 0, len(versions))
	for i := range versions {
		ids = append(ids, versions[i].AuthorId)
	}
	var users []struct {
		Id       int
		Username string
	}
	if len(ids) == 0 || DB.Table("users").Select("id,username").Where("id IN ?", ids).Find(&users).Error != nil {
		return
	}
	names := make(map[int]string, len(users))
	for _, user := range users {
		names[user.Id] = user.Username
	}
	for i := range versions {
		versions[i].AuthorUsername = names[versions[i].AuthorId]
	}
}

// RevokeScriptVersion marks a version revoked. It is idempotent-safe: revoking
// an already-revoked version updates the reason but keeps the first timestamp.
// Code, hash and signature are never modified.
func RevokeScriptVersion(scriptId int, version int, reason string, severity string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var v ScriptVersion
		if err := tx.Where("script_id = ? AND version = ?", scriptId, version).First(&v).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrScriptVersionNotFound
			}
			return err
		}
		updates := map[string]any{
			"review_status":   ScriptVersionRevoked,
			"revoked_reason":  reason,
			"revoke_severity": severity,
		}
		if v.RevokedAt == 0 {
			updates["revoked_at"] = common.GetTimestamp()
		}
		return tx.Model(&ScriptVersion{}).Where("id = ?", v.Id).Updates(updates).Error
	})
}
