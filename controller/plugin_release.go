package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

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
		"available":     true,
		"version":       release.Version,
		"filename":      release.Filename,
		"download_url":  release.DownloadUrl,
		"release_notes": release.ReleaseNotes,
		"updated_at":    release.CreatedAt,
	})
}

// DownloadLatestPluginRelease redirects to the external download URL of the
// newest plugin release. Public so the node console and extension can link to it.
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
	if release.DownloadUrl == "" {
		c.String(http.StatusNotFound, "no download URL configured for this release")
		return
	}
	c.Redirect(http.StatusFound, release.DownloadUrl)
}

// UploadPluginRelease (admin) registers a new plugin release by its external
// download URL. Required fields: download_url, version, filename.
// Optional: release_notes.
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

	downloadUrl := strings.TrimSpace(c.PostForm("download_url"))
	if downloadUrl == "" {
		common.ApiErrorMsg(c, "download_url is required")
		return
	}
	if len(downloadUrl) > 512 {
		common.ApiErrorMsg(c, "download_url is too long")
		return
	}

	filename := strings.TrimSpace(c.PostForm("filename"))
	if filename == "" {
		common.ApiErrorMsg(c, "filename is required")
		return
	}

	release := &model.PluginRelease{
		Version:      version,
		Filename:     filename,
		DownloadUrl:  downloadUrl,
		ReleaseNotes: strings.TrimSpace(c.PostForm("release_notes")),
		UploadedBy:   c.GetInt("id"),
	}
	if err := model.CreatePluginRelease(release); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"available":     true,
		"version":       release.Version,
		"filename":      release.Filename,
		"download_url":  release.DownloadUrl,
		"release_notes": release.ReleaseNotes,
		"updated_at":    release.CreatedAt,
	})
}

