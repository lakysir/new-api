package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

// DoubaoVideoProtocolConvert adapts the official content-generation protocol
// to the task request shape used by the existing video relay pipeline.
func DoubaoVideoProtocolConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			c.Request.URL.Path = "/v1/video/generations/" + c.Param("task_id")
			c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
			c.Next()
			return
		}

		var original map[string]any
		if err := common.UnmarshalBodyReusable(c, &original); err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request body")
			return
		}

		modelName, _ := original["model"].(string)
		if strings.TrimSpace(modelName) == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "model field is required")
			return
		}

		var prompts []string
		var images []string
		if content, ok := original["content"].([]any); ok {
			for _, rawItem := range content {
				item, ok := rawItem.(map[string]any)
				if !ok {
					continue
				}
				switch item["type"] {
				case "text":
					if text, _ := item["text"].(string); strings.TrimSpace(text) != "" {
						prompts = append(prompts, text)
					}
				case "image_url":
					if image, ok := item["image_url"].(map[string]any); ok {
						if url, _ := image["url"].(string); strings.TrimSpace(url) != "" {
							images = append(images, url)
						}
					}
				}
			}
		}

		prompt := strings.Join(prompts, "\n")
		if strings.TrimSpace(prompt) == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "text content is required")
			return
		}

		unified := map[string]any{
			"model":    modelName,
			"prompt":   prompt,
			"metadata": original,
		}
		if len(images) > 0 {
			unified["images"] = images
		}
		if duration, ok := original["duration"]; ok {
			unified["duration"] = duration
		}

		body, err := common.Marshal(unified)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to convert request body")
			return
		}

		common.CleanupBodyStorage(c)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.KeyRequestBody, body)
		c.Request.URL.Path = "/v1/video/generations"
		c.Set("relay_mode", relayconstant.RelayModeVideoSubmit)
		c.Next()
	}
}

type doubaoBufferedResponseWriter struct {
	gin.ResponseWriter
	body       bytes.Buffer
	statusCode int
}

func (w *doubaoBufferedResponseWriter) WriteHeader(code int) {
	if w.statusCode == 0 {
		w.statusCode = code
	}
}

func (w *doubaoBufferedResponseWriter) WriteHeaderNow() {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
}

func (w *doubaoBufferedResponseWriter) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	return w.body.Write(data)
}

func (w *doubaoBufferedResponseWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *doubaoBufferedResponseWriter) Status() int {
	if w.statusCode == 0 {
		return http.StatusOK
	}
	return w.statusCode
}

func (w *doubaoBufferedResponseWriter) Size() int {
	return w.body.Len()
}

func (w *doubaoBufferedResponseWriter) Written() bool {
	return w.statusCode != 0
}

// DoubaoVideoCreateResponseConvert hides the provider-specific create response
// and returns the public task ID in the Volcengine-compatible shape.
func DoubaoVideoCreateResponseConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		originalWriter := c.Writer
		bufferedWriter := &doubaoBufferedResponseWriter{ResponseWriter: originalWriter}
		c.Writer = bufferedWriter

		c.Next()
		c.Writer = originalWriter

		statusCode := bufferedWriter.Status()
		responseBody := bufferedWriter.body.Bytes()
		publicTaskID := common.GetContextKeyString(c, constant.ContextKeyPublicTaskID)
		if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices && publicTaskID != "" {
			var err error
			responseBody, err = common.Marshal(map[string]string{"id": publicTaskID})
			if err != nil {
				statusCode = http.StatusInternalServerError
				responseBody = []byte(`{"error":{"message":"Failed to convert response"}}`)
			}
		}

		originalWriter.Header().Del("Content-Length")
		originalWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
		originalWriter.WriteHeader(statusCode)
		_, _ = originalWriter.Write(responseBody)
	}
}
