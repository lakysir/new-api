package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetInvoiceOverview(c *gin.Context) {
	overview, err := model.GetInvoiceOverview(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": overview})
}

func CreateInvoiceApplication(c *gin.Context) {
	var application model.InvoiceApplication
	if err := common.DecodeJson(c.Request.Body, &application); err != nil {
		common.ApiError(c, err)
		return
	}
	application.Remark = strings.TrimSpace(application.Remark)
	if len(application.Title) > 200 || len(application.TaxNumber) > 64 || len(application.Email) > 128 || len(application.Remark) > 500 {
		common.ApiError(c, strconv.ErrSyntax)
		return
	}
	if err := model.CreateInvoiceApplication(c.GetInt("id"), &application); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": application})
}

func CancelInvoiceApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id")); if err != nil { common.ApiError(c, err); return }
	if err := model.CancelInvoiceApplication(c.GetInt("id"), id); err != nil { common.ApiError(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func ListInvoiceProfiles(c *gin.Context) {
	profiles, err := model.ListInvoiceProfiles(c.GetInt("id"))
	if err != nil { common.ApiError(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": profiles})
}

func SaveInvoiceProfile(c *gin.Context) {
	var profile model.InvoiceProfile
	if err := common.DecodeJson(c.Request.Body, &profile); err != nil { common.ApiError(c, err); return }
	if err := model.SaveInvoiceProfile(c.GetInt("id"), &profile); err != nil { common.ApiError(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": profile})
}

func DeleteInvoiceProfile(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id")); if err != nil { common.ApiError(c, err); return }
	if err := model.DeleteInvoiceProfile(c.GetInt("id"), id); err != nil { common.ApiError(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func AdminListInvoiceApplications(c *gin.Context) {
	items, err := model.ListInvoiceApplications(strings.TrimSpace(c.Query("status")))
	if err != nil { common.ApiError(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

type invoiceReviewRequest struct { Approved bool `json:"approved"`; Reason string `json:"reason"` }

func AdminReviewInvoiceApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id")); if err != nil { common.ApiError(c, err); return }
	var req invoiceReviewRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil { common.ApiError(c, err); return }
	if err := model.ReviewInvoiceApplication(id, c.GetInt("id"), req.Approved, req.Reason); err != nil { common.ApiError(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func AdminMarkInvoiceSent(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id")); if err != nil { common.ApiError(c, err); return }
	if err := model.MarkInvoiceSent(id, c.GetInt("id")); err != nil { common.ApiError(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true})
}
