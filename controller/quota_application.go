package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type createQuotaApplicationRequest struct {
	TargetUserId       int    `json:"target_user_id"`
	QuotaAmount        int    `json:"quota_amount"`
	InvoiceAmountCents int64  `json:"invoice_amount_cents"`
	ApplicationInfo    string `json:"application_info"`
}

type reviewQuotaApplicationRequest struct {
	Approved bool   `json:"approved"`
	Comment  string `json:"comment"`
}

func quotaApplicationScope(c *gin.Context) *int {
	if c.GetInt("role") == common.RoleRootUser {
		return nil
	}
	applicantId := c.GetInt("id")
	return &applicantId
}

func ListQuotaApplications(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !model.IsQuotaApplicationStatus(status) {
		common.ApiError(c, strconv.ErrSyntax)
		return
	}
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListQuotaApplications(
		quotaApplicationScope(c),
		status,
		strings.TrimSpace(c.Query("keyword")),
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"items": items, "total": total, "page": pageInfo.Page, "page_size": pageInfo.PageSize,
	}})
}

func GetQuotaApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	application, err := model.GetQuotaApplication(id, quotaApplicationScope(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": application})
}

func CreateQuotaApplication(c *gin.Context) {
	var req createQuotaApplicationRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	application := &model.QuotaApplication{
		ApplicantId:        c.GetInt("id"),
		TargetUserId:       req.TargetUserId,
		QuotaAmount:        req.QuotaAmount,
		InvoiceAmountCents: req.InvoiceAmountCents,
		ApplicationInfo:    req.ApplicationInfo,
	}
	if err := model.CreateQuotaApplication(application); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, application.TargetUserId, "quota_application.create", map[string]interface{}{
		"application_id": application.Id,
		"quota":          application.QuotaAmount,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": application})
}

func SearchQuotaApplicationTargets(c *gin.Context) {
	users, err := model.SearchQuotaApplicationTargets(c.Query("keyword"), 20)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": users})
}

func ReviewQuotaApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req reviewQuotaApplicationRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	application, err := model.ReviewQuotaApplication(id, c.GetInt("id"), req.Approved, req.Comment)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Approved {
		if err := model.InvalidateUserCache(application.TargetUserId); err != nil {
			common.SysLog("failed to invalidate user cache after quota application approval: " + err.Error())
		}
	}
	recordManageAuditFor(c, application.TargetUserId, "quota_application.review", map[string]interface{}{
		"application_id": application.Id,
		"approved":       req.Approved,
		"quota":          application.QuotaAmount,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": application})
}
