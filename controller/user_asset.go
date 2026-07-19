package controller

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const defaultAssetUploadMaxMB = 100

type assetStorage struct {
	endpoint    string
	bucket      string
	publicURL   string
	credentials aws.CredentialsProvider
}

func newAssetStorage() (*assetStorage, error) {
	endpoint := strings.TrimRight(common.GetEnvOrDefaultString("R2_ENDPOINT", "https://1bc2faa71bc17fb0d526aeaac107cf27.r2.cloudflarestorage.com"), "/")
	bucket := strings.TrimSpace(common.GetEnvOrDefaultString("R2_BUCKET", "newtoken"))
	publicURL := strings.TrimRight(common.GetEnvOrDefaultString("R2_PUBLIC_URL", "https://pub-9ad7d8a1c82943daa4f742b3dc1fdf61.r2.dev"), "/")
	accessKey := strings.TrimSpace(common.GetEnvOrDefaultString("R2_ACCESS_KEY_ID", ""))
	secretKey := strings.TrimSpace(common.GetEnvOrDefaultString("R2_SECRET_ACCESS_KEY", ""))
	missing := make([]string, 0, 5)
	for name, value := range map[string]string{
		"R2_ENDPOINT":          endpoint,
		"R2_BUCKET":            bucket,
		"R2_PUBLIC_URL":        publicURL,
		"R2_ACCESS_KEY_ID":     accessKey,
		"R2_SECRET_ACCESS_KEY": secretKey,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("R2 storage is not configured; missing runtime environment variables: %s", strings.Join(missing, ", "))
	}
	return &assetStorage{
		endpoint:    endpoint,
		bucket:      bucket,
		publicURL:   publicURL,
		credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	}, nil
}

func (storage *assetStorage) request(c *gin.Context, method, objectKey, contentType string, body []byte) error {
	target := storage.endpoint + "/" + url.PathEscape(storage.bucket) + "/" + assetEscapedPath(objectKey)
	request, err := http.NewRequestWithContext(c.Request.Context(), method, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	payloadHashBytes := sha256.Sum256(body)
	payloadHash := fmt.Sprintf("%x", payloadHashBytes)
	// R2 requires the x-amz-content-sha256 header on every signed request. The
	// standalone SigV4 signer does not add it automatically (unlike the S3 SDK
	// client middleware), so set it before signing so it is included in the
	// signed headers.
	request.Header.Set("X-Amz-Content-Sha256", payloadHash)
	credentialValue, err := storage.credentials.Retrieve(c.Request.Context())
	if err != nil {
		return err
	}
	if err := v4.NewSigner().SignHTTP(c.Request.Context(), credentialValue, request, payloadHash, "s3", "auto", time.Now()); err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("R2 returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

func ListUserAssets(c *gin.Context) {
	assets, err := model.ListUserAssets(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, assets)
}

func UploadUserAsset(c *gin.Context) {
	maxBytes := int64(common.GetEnvOrDefault("R2_MAX_UPLOAD_MB", defaultAssetUploadMaxMB)) << 20
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+(1<<20))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.ApiErrorMsg(c, "file is required")
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > maxBytes {
		common.ApiErrorMsg(c, fmt.Sprintf("file must be between 1 byte and %d MB", maxBytes>>20))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer file.Close()

	header := make([]byte, 512)
	n, readErr := io.ReadFull(file, header)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		common.ApiError(c, readErr)
		return
	}
	detectedType := http.DetectContentType(header[:n])
	mediaType := strings.SplitN(detectedType, "/", 2)[0]
	if mediaType != "image" && mediaType != "video" && mediaType != "audio" {
		common.ApiErrorMsg(c, "only image, video, and audio files are supported")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		common.ApiError(c, err)
		return
	}

	storage, err := newAssetStorage()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	extension := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if extension == "" {
		if extensions, extErr := mime.ExtensionsByType(detectedType); extErr == nil && len(extensions) > 0 {
			extension = extensions[0]
		}
	}
	objectKey := fmt.Sprintf("users/%d/%s/%s%s", c.GetInt("id"), mediaType, uuid.NewString(), extension)
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := storage.request(c, http.MethodPut, objectKey, detectedType, content); err != nil {
		common.ApiError(c, fmt.Errorf("upload to R2 failed: %w", err))
		return
	}

	asset := &model.UserAsset{
		UserId:      c.GetInt("id"),
		ObjectKey:   objectKey,
		Filename:    filepath.Base(fileHeader.Filename),
		ContentType: detectedType,
		MediaType:   mediaType,
		Size:        fileHeader.Size,
		PublicUrl:   assetPublicURL(storage.publicURL, objectKey),
	}
	if err := model.CreateUserAsset(asset); err != nil {
		_ = storage.request(c, http.MethodDelete, objectKey, "", nil)
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, asset)
}

func DeleteUserAsset(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid asset id")
		return
	}
	asset, err := model.GetUserAsset(c.GetInt("id"), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "asset not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	storage, err := newAssetStorage()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := storage.request(c, http.MethodDelete, asset.ObjectKey, "", nil); err != nil {
		common.ApiError(c, fmt.Errorf("delete from R2 failed: %w", err))
		return
	}
	if err := model.DeleteUserAsset(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// Keep URL construction explicit and escaped without exposing storage credentials.
func assetPublicURL(baseURL, objectKey string) string {
	return strings.TrimRight(baseURL, "/") + "/" + assetEscapedPath(objectKey)
}

func assetEscapedPath(objectKey string) string {
	parts := strings.Split(objectKey, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}
