package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoubaoVideoProtocolConvertPreservesOfficialMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v3/contents/generations/tasks",
		DoubaoVideoProtocolConvert(),
		DoubaoVideoCreateResponseConvert(),
		func(c *gin.Context) {
			assert.Equal(t, "/v1/video/generations", c.Request.URL.Path)
			var body struct {
				Model    string         `json:"model"`
				Prompt   string         `json:"prompt"`
				Images   []string       `json:"images"`
				Duration int            `json:"duration"`
				Metadata map[string]any `json:"metadata"`
			}
			require.NoError(t, common.UnmarshalBodyReusable(c, &body))
			assert.Equal(t, "doubao-seedance-2-0-260128", body.Model)
			assert.Equal(t, "first line\nsecond line", body.Prompt)
			assert.Equal(t, []string{"https://example.com/input.png"}, body.Images)
			assert.Equal(t, 6, body.Duration)
			assert.Equal(t, "1080p", body.Metadata["resolution"])
			content, ok := body.Metadata["content"].([]any)
			require.True(t, ok)
			assert.Len(t, content, 4)

			common.SetContextKey(c, constant.ContextKeyPublicTaskID, "task_public")
			c.JSON(http.StatusOK, gin.H{"id": "upstream_secret"})
		})

	body := `{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"first line"},
			{"type":"image_url","image_url":{"url":"https://example.com/input.png"}},
			{"type":"video_url","video_url":{"url":"https://example.com/input.mp4"}},
			{"type":"text","text":"second line"}
		],
		"duration":6,
		"resolution":"1080p"
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response map[string]string
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, map[string]string{"id": "task_public"}, response)
}

func TestDoubaoVideoCreateResponseConvertPreservesErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v3/contents/generations/tasks",
		DoubaoVideoCreateResponseConvert(),
		func(c *gin.Context) {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "invalid"}})
		})

	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.JSONEq(t, `{"error":{"message":"invalid"}}`, recorder.Body.String())
}

func TestDoubaoVideoProtocolConvertRewritesFetchPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v3/contents/generations/tasks/:task_id",
		DoubaoVideoProtocolConvert(),
		func(c *gin.Context) {
			assert.Equal(t, "/v1/video/generations/task_public", c.Request.URL.Path)
			assert.Equal(t, "task_public", c.Param("task_id"))
			c.Status(http.StatusNoContent)
		})

	request := httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_public", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}
