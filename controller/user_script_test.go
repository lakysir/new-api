package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type publishedScriptAPIResponse struct {
	Success bool                    `json:"success"`
	Message string                  `json:"message"`
	Data    publishedScriptPageData `json:"data"`
}

type publishedScriptPageData struct {
	Items    []publishedScriptListItem `json:"items"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

func setupUserScriptControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	if err := db.AutoMigrate(&model.UserScript{}, &model.ScriptVersion{}); err != nil {
		t.Fatalf("failed to migrate user scripts: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedUserScript(t *testing.T, db *gorm.DB, title string, published bool, updatedAt int64) model.UserScript {
	t.Helper()
	script := model.UserScript{
		UserId:        1,
		Title:         title,
		Description:   title + " description",
		ScriptParams:  `{"prompt":"hello"}`,
		DraftCode:     "draft-secret-" + title,
		PublishedCode: "published-secret-" + title,
		Published:     published,
		PublishedAt:   updatedAt,
		CreatedAt:     updatedAt,
		UpdatedAt:     updatedAt,
	}
	if err := db.Create(&script).Error; err != nil {
		t.Fatalf("failed to seed script: %v", err)
	}
	if published {
		script.LatestVersion = 1
		if err := db.Model(&script).Update("latest_version", script.LatestVersion).Error; err != nil {
			t.Fatalf("failed to set latest script version: %v", err)
		}
		version := model.ScriptVersion{
			ScriptId:     script.Id,
			AuthorId:     script.UserId,
			Version:      1,
			Title:        script.Title,
			Description:  script.Description,
			Code:         script.PublishedCode,
			ReviewStatus: model.ScriptVersionApproved,
			PublishedAt:  updatedAt,
		}
		if err := db.Create(&version).Error; err != nil {
			t.Fatalf("failed to seed script version: %v", err)
		}
	}
	return script
}

func requestPublishedScripts(t *testing.T, target string) (*httptest.ResponseRecorder, publishedScriptAPIResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	ApiListPublishedScripts(ctx)

	var response publishedScriptAPIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v; body=%s", err, recorder.Body.String())
	}
	return recorder, response
}

func TestApiListPublishedScriptsReturnsOnlyPublishedMetadata(t *testing.T) {
	db := setupUserScriptControllerTestDB(t)
	older := seedUserScript(t, db, "older", true, 10)
	newer := seedUserScript(t, db, "newer", true, 20)
	seedUserScript(t, db, "draft", false, 30)
	if err := db.Model(&newer).Updates(map[string]any{
		"title": "edited draft title", "description": "edited draft description",
	}).Error; err != nil {
		t.Fatalf("failed to edit published script draft: %v", err)
	}

	recorder, response := requestPublishedScripts(t, "/api/script-api/scripts/published?page=1&page_size=20")
	if !response.Success {
		t.Fatalf("expected success response, got %q", response.Message)
	}
	if response.Data.Total != 2 || len(response.Data.Items) != 2 {
		t.Fatalf("expected two published scripts, total=%d items=%d", response.Data.Total, len(response.Data.Items))
	}
	if response.Data.Items[0].Id != newer.Id || response.Data.Items[1].Id != older.Id {
		t.Fatalf("expected scripts ordered newest first, got %+v", response.Data.Items)
	}
	if response.Data.Items[0].Title != "newer" || response.Data.Items[0].Description != "newer description" {
		t.Fatalf("square metadata must come from the published version: %+v", response.Data.Items[0])
	}
	body := recorder.Body.String()
	for _, secret := range []string{"draft-secret", "published-secret", "code_preview", "published_code", "draft_code"} {
		if strings.Contains(body, secret) {
			t.Fatalf("published script list leaked %q: %s", secret, body)
		}
	}
}

func TestApiListPublishedScriptsPaginatesAndCapsPageSize(t *testing.T) {
	db := setupUserScriptControllerTestDB(t)
	for i := 1; i <= 3; i++ {
		seedUserScript(t, db, fmt.Sprintf("script-%d", i), true, int64(i))
	}

	_, secondPage := requestPublishedScripts(t, "/api/script-api/scripts/published?page=2&page_size=1")
	if secondPage.Data.Page != 2 || secondPage.Data.PageSize != 1 || secondPage.Data.Total != 3 || len(secondPage.Data.Items) != 1 {
		t.Fatalf("unexpected second page response: %+v", secondPage.Data)
	}
	if secondPage.Data.Items[0].Title != "script-2" {
		t.Fatalf("expected script-2 on second page, got %+v", secondPage.Data.Items[0])
	}

	_, capped := requestPublishedScripts(t, "/api/script-api/scripts/published?page=1&page_size=999")
	if capped.Data.PageSize != 100 {
		t.Fatalf("expected page size capped at 100, got %d", capped.Data.PageSize)
	}
}

func TestApiListPublishedScriptsExcludesLegacyUnreviewedPublish(t *testing.T) {
	db := setupUserScriptControllerTestDB(t)
	legacy := seedUserScript(t, db, "legacy", false, 10)
	if err := db.Model(&legacy).Update("published", true).Error; err != nil {
		t.Fatalf("failed to mark legacy script published: %v", err)
	}

	_, response := requestPublishedScripts(t, "/api/script-api/scripts/published?page=1&page_size=20")
	if !response.Success || response.Data.Total != 0 || len(response.Data.Items) != 0 {
		t.Fatalf("legacy unreviewed publish must not enter square: %+v", response)
	}
}
