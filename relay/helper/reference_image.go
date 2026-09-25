package helper

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func CountReferenceImages(body []byte) int {
	var root map[string]json.RawMessage
	if len(body) == 0 || json.Unmarshal(body, &root) != nil { return 0 }
	countValue := func(raw json.RawMessage) int {
		if len(raw) == 0 || string(raw) == "null" { return 0 }
		var array []json.RawMessage
		if json.Unmarshal(raw, &array) == nil { return len(array) }
		var value string
		if json.Unmarshal(raw, &value) == nil && value != "" { return 1 }
		return 1
	}
	count := countValue(root["image"]) + countValue(root["images"])
	var content []map[string]json.RawMessage
	if json.Unmarshal(root["content"], &content) == nil {
		for _, item := range content {
			var typ string
			_ = json.Unmarshal(item["type"], &typ)
			if typ != "image_url" || len(item["image_url"]) == 0 { continue }
			var imageURL struct { URL string `json:"url"` }
			if json.Unmarshal(item["image_url"], &imageURL) == nil {
				if imageURL.URL != "" { count++ }
			} else {
				var imageURLString string
				if json.Unmarshal(item["image_url"], &imageURLString) == nil && imageURLString != "" { count++ }
			}
		}
	}
	return count
}

func ApplyReferenceImagePricing(c *gin.Context, info *relaycommon.RelayInfo, body []byte) {
	if info == nil { return }
	if !info.PriceData.UsePrice { return }
	config, ok := ratio_setting.GetReferenceImagePricing(info.OriginModelName)
	if !ok || config.FreeCount <= 0 || config.UnitPrice <= 0 { return }
	if len(body) == 0 {
		if storage, err := common.GetBodyStorage(c); err == nil { body, _ = storage.Bytes() }
	}
	count := CountReferenceImages(body)
	if count <= config.FreeCount { return }
	quota := int(float64(count-config.FreeCount) * config.UnitPrice * common.QuotaPerUnit * info.PriceData.GroupRatioInfo.GroupRatio)
	if quota <= 0 { return }
	info.PriceData.ReferenceImageQuota += quota
	info.PriceData.FreeModel = false
	if info.PriceData.UsePrice {
		info.PriceData.QuotaToPreConsume += quota
	}
}
