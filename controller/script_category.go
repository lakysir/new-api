package controller

import (
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/nodehub"
	"github.com/gin-gonic/gin"
)

// balanceCheckTTL is how long a passing balance probe keeps a category usable
// on a node before it must be re-probed.
const balanceCheckTTL = 6 * time.Hour

// ListCategories returns all script categories (public — clients/providers use
// it to browse by target site).
func ListCategories(c *gin.Context) {
	cats, err := model.ListScriptCategories()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, cats)
}

type createCategoryRequest struct {
	Name string `json:"name"`
	Site string `json:"site"`
}

// CreateCategory creates a target-site category (operator only).
func CreateCategory(c *gin.Context) {
	var req createCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Name == "" {
		common.ApiErrorMsg(c, "name is required")
		return
	}
	cat := &model.ScriptCategory{Name: req.Name, Site: req.Site}
	if err := model.CreateScriptCategory(cat); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, cat)
}

type setBalanceScriptRequest struct {
	ScriptId int `json:"script_id"`
	Version  int `json:"version"`
}

// SetCategoryBalanceScript designates a category's audited balance-probe script
// (operator only). The script+version must be executable.
func SetCategoryBalanceScript(c *gin.Context) {
	categoryId, err := strconv.Atoi(c.Param("id"))
	if err != nil || categoryId <= 0 {
		common.ApiErrorMsg(c, "invalid category id")
		return
	}
	var req setBalanceScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetCategoryBalanceScript(categoryId, req.ScriptId, req.Version); err != nil {
		if errors.Is(err, model.ErrCategoryNotFound) {
			common.ApiErrorMsg(c, "category not found")
			return
		}
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, nil)
}

type balanceCheckRequest struct {
	CategoryId    int    `json:"category_id"`
	BalanceOk     bool   `json:"balance_ok"`
	BalanceMicros int64  `json:"balance_micros"`
	Tier          string `json:"tier"`
	ErrorMessage  string `json:"error_message"`
}

func ownedNode(c *gin.Context, nodeId string) (*model.Node, bool) {
	node, err := model.GetNode(nodeId)
	if err != nil || node.UserId != c.GetInt("id") {
		common.ApiErrorMsg(c, "node not found")
		return nil, false
	}
	return node, true
}

// RequestBalanceCheck sends a free control-plane probe command to the owner's
// live plugin. It creates no paid order, lease, price snapshot or ledger entry.
func RequestBalanceCheck(c *gin.Context) {
	nodeId := c.Param("id")
	if _, ok := ownedNode(c, nodeId); !ok {
		return
	}
	var req struct {
		CategoryId int `json:"category_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.CategoryId <= 0 {
		common.ApiErrorMsg(c, "category_id is required")
		return
	}
	cat, err := model.GetScriptCategory(req.CategoryId)
	if err != nil || cat.BalanceScriptId <= 0 || cat.BalanceScriptVersion <= 0 {
		common.ApiErrorMsg(c, "category has no balance-probe script")
		return
	}
	if _, err := model.GetExecutableScriptVersion(cat.BalanceScriptId, cat.BalanceScriptVersion); err != nil {
		common.ApiErrorMsg(c, "balance-probe script is not executable")
		return
	}
	eventId := model.NewEventId()
	if err := nodehub.Default.Send(nodeId, gin.H{
		"type": "balance.check", "event_id": eventId, "node_id": nodeId,
		"category_id": req.CategoryId, "script_id": cat.BalanceScriptId,
		"script_version": cat.BalanceScriptVersion, "target_site": cat.Site,
	}); err != nil {
		common.ApiErrorMsg(c, "node is not connected")
		return
	}
	common.ApiSuccess(c, gin.H{"event_id": eventId, "dispatched": true})
}

func ListBalanceChecks(c *gin.Context) {
	nodeId := c.Param("id")
	if _, ok := ownedNode(c, nodeId); !ok {
		return
	}
	statuses, err := model.ListNodeSiteStatuses(nodeId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, statuses)
}

// ReportBalanceCheck records a node's balance-probe result for a category. The
// plugin runs the category's balance-probe script (reads the site balance,
// no generation) and reports here; a passing result grants a window during
// which the node may list capabilities in that category.
func ReportBalanceCheck(c *gin.Context) {
	nodeId := c.Param("id")
	if _, ok := ownedNode(c, nodeId); !ok {
		return
	}
	var req balanceCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.CategoryId <= 0 {
		common.ApiErrorMsg(c, "category_id is required")
		return
	}
	now := time.Now().UnixMilli()
	expiresAt := int64(0)
	if req.BalanceOk {
		expiresAt = time.Now().Add(balanceCheckTTL).Unix()
	}
	status := &model.NodeSiteStatus{
		NodeId:        nodeId,
		CategoryId:    req.CategoryId,
		UserId:        c.GetInt("id"),
		BalanceOk:     req.BalanceOk,
		BalanceMicros: req.BalanceMicros,
		Tier:          req.Tier,
		ErrorMessage:  req.ErrorMessage,
		CheckedAt:     now,
		ExpiresAt:     expiresAt,
	}
	if err := model.RecordBalanceCheck(status); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}
