package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type upstreamModelMetadata struct {
	ModelName               string   `json:"model_name"`
	Description             string   `json:"description,omitempty"`
	DescriptionEn           string   `json:"description_en,omitempty"`
	Icon                    string   `json:"icon,omitempty"`
	Tags                    string   `json:"tags,omitempty"`
	Endpoints               []string `json:"supported_endpoint_types,omitempty"`
	EndpointsConfig         string   `json:"endpoints,omitempty"`
	RequestPriceUnits       int      `json:"request_price_units,omitempty"`
	RequestPriceDisplayUnit string   `json:"request_price_display_unit,omitempty"`
	QuotaType               int      `json:"quota_type"`
	ModelRatio              float64  `json:"model_ratio,omitempty"`
	ModelPrice              float64  `json:"model_price,omitempty"`
	CompletionRatio         float64  `json:"completion_ratio,omitempty"`
	CacheRatio              *float64 `json:"cache_ratio,omitempty"`
	CreateCacheRatio        *float64 `json:"create_cache_ratio,omitempty"`
	ImageRatio              *float64 `json:"image_ratio,omitempty"`
	AudioRatio              *float64 `json:"audio_ratio,omitempty"`
	AudioCompletionRatio    *float64 `json:"audio_completion_ratio,omitempty"`
	VendorName              string   `json:"vendor_name,omitempty"`
	VendorIcon              string   `json:"vendor_icon,omitempty"`
	VendorID                int      `json:"vendor_id,omitempty"`
	SourceChannel           string   `json:"source_channel,omitempty"`
	Source                   string   `json:"source"`
}

type pricingModelEnvelope struct {
	Success bool                  `json:"success"`
	Data    upstreamModelMetadata `json:"data"`
	Vendor  *model.PricingVendor  `json:"vendor"`
}

type pricingListEnvelope struct {
	Success bool                    `json:"success"`
	Data    []upstreamModelMetadata `json:"data"`
	Vendors []model.PricingVendor   `json:"vendors"`
}

func channelsForUpstreamModel(modelName string) ([]model.Channel, error) {
	var channels []model.Channel
	err := model.DB.Table("channels").
		Select("channels.*").
		Joins("JOIN abilities ON abilities.channel_id = channels.id").
		Where("abilities.model = ? AND abilities.enabled = ? AND channels.status = ?", modelName, true, common.ChannelStatusEnabled).
		Order("COALESCE(abilities.priority, 0) DESC").
		Order("abilities.weight DESC").
		Find(&channels).Error
	return channels, err
}

func fetchChannelModelMetadata(channel *model.Channel, modelName string) (*upstreamModelMetadata, error) {
	baseURL := strings.TrimRight(channel.GetBaseURL(), "/")
	keys := channel.GetKeys()
	if baseURL == "" || len(keys) == 0 {
		return nil, fmt.Errorf("channel has no upstream address or key")
	}
	key := strings.TrimSpace(keys[0])
	headers, err := buildFetchModelsHeaders(channel, key)
	if err != nil {
		return nil, err
	}

	requestURL := baseURL + "/v1/model-metadata?model=" + url.QueryEscape(modelName)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	resp, err := newHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		var envelope pricingModelEnvelope
		if json.NewDecoder(resp.Body).Decode(&envelope) == nil && envelope.Success && envelope.Data.ModelName == modelName {
			if envelope.Vendor != nil {
				envelope.Data.VendorName = envelope.Vendor.Name
				envelope.Data.VendorIcon = envelope.Vendor.Icon
			}
			envelope.Data.Source = "channel_metadata"
			envelope.Data.SourceChannel = channel.Name
			return &envelope.Data, nil
		}
	}

	listReq, err := http.NewRequest(http.MethodGet, baseURL+"/api/pricing", nil)
	if err != nil {
		return nil, err
	}
	listReq.Header = headers
	listResp, err := newHTTPClient().Do(listReq)
	if err != nil {
		return nil, err
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream pricing endpoint returned %d", listResp.StatusCode)
	}
	var list pricingListEnvelope
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		return nil, err
	}
	for _, item := range list.Data {
		if item.ModelName != modelName {
			continue
		}
		item.Source = "channel_pricing"
		item.SourceChannel = channel.Name
		for _, vendor := range list.Vendors {
			if vendor.ID == item.VendorID {
				item.VendorName = vendor.Name
				item.VendorIcon = vendor.Icon
				break
			}
		}
		return &item, nil
	}
	return nil, fmt.Errorf("upstream pricing did not contain model metadata")
}

func GetUpstreamModelMetadata(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" {
		common.ApiErrorMsg(c, "model is required")
		return
	}
	channels, err := channelsForUpstreamModel(modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var lastErr error
	seenChannels := make(map[int]struct{}, len(channels))
	for i := range channels {
		if _, seen := seenChannels[channels[i].Id]; seen {
			continue
		}
		seenChannels[channels[i].Id] = struct{}{}
		metadata, fetchErr := fetchChannelModelMetadata(&channels[i], modelName)
		if fetchErr == nil {
			common.ApiSuccess(c, metadata)
			return
		}
		lastErr = fetchErr
	}
	message := "No supporting channel returned complete model metadata"
	if lastErr != nil {
		message += ": " + lastErr.Error()
	}
	c.JSON(http.StatusOK, gin.H{"success": false, "message": message})
}
