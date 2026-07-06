package model

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"
)

const UserScriptMaxCodeLength = 1024 * 1024

type UserScript struct {
	Id               int            `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId           int            `json:"user_id" gorm:"index;not null"`
	Title            string         `json:"title" gorm:"type:varchar(128);not null"`
	Description      string         `json:"description" gorm:"type:text"`
	ScriptParams     string         `json:"script_params" gorm:"type:text"`
	DraftCode        string         `json:"draft_code,omitempty" gorm:"type:text"`
	PublishedCode    string         `json:"published_code,omitempty" gorm:"type:text"`
	Published        bool           `json:"published" gorm:"index"`
	PublishedAt      int64          `json:"published_at" gorm:"bigint;default:0"`
	CreatedAt        int64          `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt        int64          `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
	CodePreview      string         `json:"code_preview,omitempty" gorm:"-"`
	PreviewTruncated bool           `json:"preview_truncated,omitempty" gorm:"-"`
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
	if err := DB.Model(&UserScript{}).Where("published = ?", true).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var scripts []UserScript
	err := DB.Select("id,user_id,title,description,script_params,published,published_at,created_at,updated_at,published_code").
		Where("published = ?", true).
		Order("updated_at desc,id desc").
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
	err := DB.Where("id = ? AND published = ?", id, true).First(&script).Error
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
	err := DB.Select("id,user_id,title,description,script_params,published,published_at,created_at,updated_at,published_code").
		Where("id = ? AND published = ?", id, true).
		First(&script).Error
	if err != nil {
		return nil, err
	}
	return &script, nil
}

func ListUserScripts(userId int) ([]UserScript, error) {
	var scripts []UserScript
	err := DB.Select("id,user_id,title,description,script_params,published,published_at,created_at,updated_at").
		Where("user_id = ?", userId).
		Order("updated_at desc,id desc").
		Find(&scripts).Error
	if err != nil {
		return nil, err
	}
	return scripts, nil
}

func GetUserScriptById(id int, userId int) (*UserScript, error) {
	var script UserScript
	err := DB.Where("id = ? AND user_id = ?", id, userId).First(&script).Error
	if err != nil {
		return nil, err
	}
	return &script, nil
}

func UpsertUserScriptDraft(userId int, id int, title string, description string, scriptParams string, code string) (*UserScript, error) {
	title, description, scriptParams, code, err := NormalizeUserScriptInput(title, description, scriptParams, code)
	if err != nil {
		return nil, err
	}
	if id > 0 {
		script, err := GetUserScriptById(id, userId)
		if err != nil {
			return nil, err
		}
		script.Title = title
		script.Description = description
		script.ScriptParams = scriptParams
		script.DraftCode = code
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
	}
	if err := DB.Create(script).Error; err != nil {
		return nil, err
	}
	return script, nil
}
