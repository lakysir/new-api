package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type userScriptSaveRequest struct {
	Id           int    `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	ScriptParams string `json:"script_params"`
	Code         string `json:"code"`
	DraftCode    string `json:"draft_code"`
}

type publishedScriptListItem struct {
	Id           int    `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	ScriptParams string `json:"script_params"`
	PublishedAt  int64  `json:"published_at"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

func parseScriptId(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid script id")
		return 0, false
	}
	return id, true
}

func scriptCodeFromRequest(req userScriptSaveRequest) string {
	if req.Code != "" {
		return req.Code
	}
	return req.DraftCode
}

func ListScriptSquare(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	scripts, total, err := model.ListPublishedUserScripts(offset, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items": scripts,
		"total": total,
	})
}

func GetScriptSquareDetail(c *gin.Context) {
	id, ok := parseScriptId(c)
	if !ok {
		return
	}
	script, err := model.GetPublishedUserScript(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "script not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, script)
}

func ListMyScripts(c *gin.Context) {
	scripts, err := model.ListUserScripts(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for i := range scripts {
		matches, matchErr := draftMatchesLatestVersion(&scripts[i])
		scripts[i].HasUnpublishedChanges = matchErr != nil || !matches
	}
	common.ApiSuccess(c, scripts)
}

func GetMyScript(c *gin.Context) {
	id, ok := parseScriptId(c)
	if !ok {
		return
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
	common.ApiSuccess(c, script)
}

func SaveMyScriptDraft(c *gin.Context) {
	var req userScriptSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	id := req.Id
	if pathId := c.Param("id"); pathId != "" {
		parsed, err := strconv.Atoi(pathId)
		if err != nil || parsed <= 0 {
			common.ApiErrorMsg(c, "invalid script id")
			return
		}
		id = parsed
	}
	script, err := model.UpsertUserScriptDraft(c.GetInt("id"), id, req.Title, req.Description, req.ScriptParams, scriptCodeFromRequest(req))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, script)
}

func DeleteMyScript(c *gin.Context) {
	id, ok := parseScriptId(c)
	if !ok {
		return
	}
	err := model.DB.Where("id = ? AND user_id = ?", id, c.GetInt("id")).Delete(&model.UserScript{}).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ApiListMyScripts(c *gin.Context) {
	ListMyScripts(c)
}

func ApiListPublishedScripts(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	// Bound the offset calculation on 32-bit builds and unreasonable requests.
	if page > 1_000_000 {
		page = 1_000_000
	}

	scripts, total, err := model.ListPublishedUserScripts((page-1)*pageSize, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]publishedScriptListItem, 0, len(scripts))
	for _, script := range scripts {
		items = append(items, publishedScriptListItem{
			Id:           script.Id,
			Title:        script.Title,
			Description:  script.Description,
			ScriptParams: script.ScriptParams,
			PublishedAt:  script.PublishedAt,
			CreatedAt:    script.CreatedAt,
			UpdatedAt:    script.UpdatedAt,
		})
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func ApiGetMyScript(c *gin.Context) {
	GetMyScript(c)
}

func ApiSaveMyScriptDraft(c *gin.Context) {
	SaveMyScriptDraft(c)
}

func ApiGetPublishedScriptCode(c *gin.Context) {
	id, ok := parseScriptId(c)
	if !ok {
		return
	}
	script, err := model.GetPublishedUserScriptCode(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "script not found"})
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"id":            script.Id,
		"title":         script.Title,
		"description":   script.Description,
		"script_params": script.ScriptParams,
		"code":          script.PublishedCode,
		"published_at":  script.PublishedAt,
		"created_at":    script.CreatedAt,
		"updated_at":    script.UpdatedAt,
	})
}
