package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.QuerySummaryAll(hours, getRequestUsablePerfGroups(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: c.Query("group"),
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result.Groups = filterActiveGroups(result.Groups)
	result.Groups = filterUsableGroups(result.Groups, getRequestUsablePerfGroupSet(c))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func getRequestUserGroup(c *gin.Context) string {
	userId, exists := c.Get("id")
	if !exists {
		return ""
	}
	id, ok := userId.(int)
	if !ok {
		return ""
	}
	user, err := model.GetUserCache(id)
	if err != nil || user == nil {
		return ""
	}
	return user.Group
}

func getRequestUsablePerfGroups(c *gin.Context) []string {
	return lo.Keys(service.GetUserUsableGroups(getRequestUserGroup(c)))
}

func getRequestUsablePerfGroupSet(c *gin.Context) map[string]struct{} {
	groups := getRequestUsablePerfGroups(c)
	set := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		set[group] = struct{}{}
	}
	return set
}

func filterActiveGroups(groups []perfmetrics.GroupResult) []perfmetrics.GroupResult {
	activeRatios := ratio_setting.GetGroupRatioCopy()
	return lo.Filter(groups, func(g perfmetrics.GroupResult, _ int) bool {
		_, ok := activeRatios[g.Group]
		return ok || g.Group == "auto"
	})
}

func filterUsableGroups(groups []perfmetrics.GroupResult, usableGroups map[string]struct{}) []perfmetrics.GroupResult {
	if len(usableGroups) == 0 {
		return nil
	}
	return lo.Filter(groups, func(g perfmetrics.GroupResult, _ int) bool {
		_, ok := usableGroups[g.Group]
		return ok
	})
}
