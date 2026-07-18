package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// maxPluginUploadBytes caps a single uploaded plugin package at 5MB.
const maxPluginUploadBytes = 5 * 1024 * 1024

// GetLatestPluginRelease returns metadata for the newest uploaded plugin
// release (no file bytes). Public: the browser extension calls this to compare
// against its own manifest version and decide whether to show an update prompt.
// Returns available=false when nothing has been uploaded yet.
func GetLatestPluginRelease(c *gin.Context) {
	release, err := model.GetLatestPluginReleaseMeta()
	if err != nil {
		if err == model.ErrPluginReleaseNotFound {
			common.ApiSuccess(c, gin.H{"available": false})
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"available":  true,
		"version":    release.Version,
		"filename":   release.Filename,
		"size":       release.Size,
		"sha256":     release.Sha256,
		"updated_at": release.CreatedAt,
	})
}

// DownloadLatestPluginRelease streams the newest uploaded plugin package as a
// file download. Public so the node console and the extension can link to it
// directly.
func DownloadLatestPluginRelease(c *gin.Context) {
	release, err := model.GetLatestPluginRelease()
	if err != nil {
		if err == model.ErrPluginReleaseNotFound {
			c.String(http.StatusNotFound, "no plugin release available")
			return
		}
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	filename := release.Filename
	if filename == "" {
		filename = "plugin.zip"
	}
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/octet-stream", release.Content)
}

// UploadPluginRelease (admin) accepts a single multipart file (≤5MB) plus a
// version string and stores it as the newest release. The upload becomes the
// version the extension checks against and the node console links to.
func UploadPluginRelease(c *gin.Context) {
	version := strings.TrimSpace(c.PostForm("version"))
	if version == "" {
		common.ApiErrorMsg(c, "version is required")
		return
	}
	if len(version) > 32 {
		common.ApiErrorMsg(c, "version is too long")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.ApiErrorMsg(c, "a plugin file is required")
		return
	}
	if fileHeader.Size <= 0 {
		common.ApiErrorMsg(c, "the plugin file is empty")
		return
	}
	if fileHeader.Size > maxPluginUploadBytes {
		common.ApiErrorMsg(c, "the plugin file must not exceed 5MB")
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer src.Close()

	// LimitReader guards against a header that under-reports the real size.
	content, err := io.ReadAll(io.LimitReader(src, maxPluginUploadBytes+1))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(content) > maxPluginUploadBytes {
		common.ApiErrorMsg(c, "the plugin file must not exceed 5MB")
		return
	}

	sum := sha256.Sum256(content)
	release := &model.PluginRelease{
		Version:    version,
		Filename:   fileHeader.Filename,
		Size:       int64(len(content)),
		Content:    content,
		Sha256:     hex.EncodeToString(sum[:]),
		UploadedBy: c.GetInt("id"),
	}
	if err := model.CreatePluginRelease(release); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"version":    release.Version,
		"filename":   release.Filename,
		"size":       release.Size,
		"sha256":     release.Sha256,
		"updated_at": release.CreatedAt,
	})
}
