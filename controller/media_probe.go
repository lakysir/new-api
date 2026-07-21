package controller

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

const mediaProbeReadLimit = 512

type mediaProbeRequest struct {
	URL string `json:"url" binding:"required"`
}

func mediaKindFromContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	default:
		return ""
	}
}

// ProbeMediaURL identifies download-style media URLs without proxying their
// contents. Some providers reject HEAD but expose the MIME type on a range GET.
func ProbeMediaURL(c *gin.Context) {
	var input mediaProbeRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "url is required"})
		return
	}

	target := strings.TrimSpace(input.URL)
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid http(s) url"})
		return
	}

	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(target, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "url is not allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid url"})
		return
	}
	req.Header.Set("Range", "bytes=0-511")
	req.Header.Set("Accept", "image/*, video/*, audio/*;q=0.9, */*;q=0.1")

	client := service.GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "kind": nil})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		c.JSON(http.StatusOK, gin.H{"success": true, "kind": nil})
		return
	}

	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType == "" || contentType == "application/octet-stream" {
		buffer, _ := io.ReadAll(io.LimitReader(resp.Body, mediaProbeReadLimit))
		contentType = strings.ToLower(http.DetectContentType(buffer))
	}

	kind := mediaKindFromContentType(contentType)
	if kind == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "kind": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "kind": kind})
}
