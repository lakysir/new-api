package model

import (
	"errors"

	"gorm.io/gorm"
)

// ScriptCategory groups market scripts by their target site (e.g. "Dreamina",
// "Veo"). Every category has one balance-probe script — a published, audited
// market script that reads the site's balance/quota WITHOUT running a
// generation task. A node must pass this probe for a category before it can
// list any generation capability in that category (架构 §5.6 availability).
type ScriptCategory struct {
	Id   int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name string `json:"name" gorm:"type:varchar(64);not null"`
	Site string `json:"site" gorm:"type:varchar(128);index"` // target site / origin label
	// BalanceScriptId / BalanceScriptVersion point to the audited balance-probe
	// market script for this category (0 until an operator designates one).
	BalanceScriptId      int   `json:"balance_script_id" gorm:"default:0"`
	BalanceScriptVersion int   `json:"balance_script_version" gorm:"default:0"`
	CreatedAt            int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt            int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ScriptCategory) TableName() string { return "script_categories" }

var (
	// ErrCategoryNotFound is returned when a category id is missing.
	ErrCategoryNotFound = errors.New("script category not found")
	// ErrCategoryNoBalanceScript is returned when a category has no designated
	// balance-probe script yet.
	ErrCategoryNoBalanceScript = errors.New("category has no balance-probe script")
)

// CreateScriptCategory inserts a new category.
func CreateScriptCategory(c *ScriptCategory) error {
	return DB.Create(c).Error
}

// ListScriptCategories returns all categories.
func ListScriptCategories() ([]ScriptCategory, error) {
	var cats []ScriptCategory
	err := DB.Order("id asc").Find(&cats).Error
	return cats, err
}

// GetScriptCategory returns a category by id.
func GetScriptCategory(id int) (*ScriptCategory, error) {
	var c ScriptCategory
	if err := DB.Where("id = ?", id).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCategoryNotFound
		}
		return nil, err
	}
	return &c, nil
}

// IsBalanceProbeScript reports whether the given script id is designated as the
// balance-probe script of any category. Balance-probe scripts read a target
// site's balance without running a generation task, so they must never be
// published as callable models.
func IsBalanceProbeScript(scriptId int) (bool, error) {
	var count int64
	err := DB.Model(&ScriptCategory{}).
		Where("balance_script_id = ?", scriptId).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// SetCategoryBalanceScript designates the audited balance-probe script+version
// for a category. The version must be executable (approved + not revoked).
func SetCategoryBalanceScript(categoryId, scriptId, version int) error {
	if _, err := GetExecutableScriptVersion(scriptId, version); err != nil {
		return err
	}
	res := DB.Model(&ScriptCategory{}).Where("id = ?", categoryId).Updates(map[string]any{
		"balance_script_id":      scriptId,
		"balance_script_version": version,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrCategoryNotFound
	}
	return nil
}
