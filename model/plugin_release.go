package model

import (
	"errors"

	"gorm.io/gorm"
)

// PluginRelease is an uploaded browser-extension package that clients can
// download. The operator uploads a single packaged file (≤5MB) tagged with a
// version string; the newest upload is treated as the latest release and is
// what the extension checks against for update prompts and what the node
// console links to for download.
type PluginRelease struct {
	Id       int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Version  string `json:"version" gorm:"type:varchar(32);not null;index"`
	Filename string `json:"filename" gorm:"type:varchar(255);not null"`
	Size     int64  `json:"size" gorm:"not null"`
	// Content holds the raw packaged bytes. Kept out of JSON so metadata queries
	// never serialize the blob.
	Content    []byte `json:"-" gorm:"type:blob"`
	Sha256     string `json:"sha256" gorm:"type:varchar(64)"`
	UploadedBy int    `json:"uploaded_by"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (PluginRelease) TableName() string { return "plugin_releases" }

// ErrPluginReleaseNotFound is returned when no release has been uploaded yet.
var ErrPluginReleaseNotFound = errors.New("no plugin release available")

// CreatePluginRelease persists a new uploaded release.
func CreatePluginRelease(r *PluginRelease) error {
	return DB.Create(r).Error
}

// GetLatestPluginReleaseMeta returns the newest release WITHOUT its content
// blob, for cheap metadata reads (version check, download listing).
func GetLatestPluginReleaseMeta() (*PluginRelease, error) {
	var r PluginRelease
	err := DB.Select("id", "version", "filename", "size", "sha256", "uploaded_by", "created_at").
		Order("id desc").First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPluginReleaseNotFound
		}
		return nil, err
	}
	return &r, nil
}

// GetLatestPluginRelease returns the newest release including its content blob,
// for serving the download.
func GetLatestPluginRelease() (*PluginRelease, error) {
	var r PluginRelease
	err := DB.Order("id desc").First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPluginReleaseNotFound
		}
		return nil, err
	}
	return &r, nil
}
