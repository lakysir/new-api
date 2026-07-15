package controller

import (
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// capabilityTestTTL is how long a passing challenge test keeps a capability
// listable before it must be re-tested (architecture §22.3).
const capabilityTestTTL = 24 * time.Hour

// parseCapabilityParams reads node id (:id), script id (:scriptId) and version
// (query or default 0) from a capability route.
func parseCapabilityParams(c *gin.Context) (nodeId string, scriptId int, version int, ok bool) {
	nodeId = c.Param("id")
	if nodeId == "" {
		common.ApiErrorMsg(c, "node id is required")
		return "", 0, 0, false
	}
	sid, err := strconv.Atoi(c.Param("scriptId"))
	if err != nil || sid <= 0 {
		common.ApiErrorMsg(c, "invalid script id")
		return "", 0, 0, false
	}
	version, _ = strconv.Atoi(c.Query("version"))
	return nodeId, sid, version, true
}

type registerNodeRequest struct {
	NodeId   string `json:"node_id"`
	DeviceId string `json:"device_id"`
	Region   string `json:"region"`
	Version  string `json:"version"`
}

// RegisterNode registers or refreshes a node for the authenticated user.
func RegisterNode(c *gin.Context) {
	var req registerNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.NodeId == "" || req.DeviceId == "" {
		common.ApiErrorMsg(c, "node_id and device_id are required")
		return
	}
	node, err := model.UpsertNode(c.GetInt("id"), req.DeviceId, req.NodeId, req.Region, req.Version)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, node)
}

type heartbeatRequest struct {
	State string `json:"state"`
}

// NodeHeartbeat records presence and idle/busy state for a node.
func NodeHeartbeat(c *gin.Context) {
	nodeId := c.Param("id")
	var req heartbeatRequest
	_ = c.ShouldBindJSON(&req)
	state := req.State
	if state == "" {
		state = model.NodeStateIdle
	}
	if err := model.TouchNodePresence(nodeId, state); err != nil {
		if errors.Is(err, model.ErrNodeNotFound) {
			common.ApiErrorMsg(c, "node not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// CreateCapabilityTest issues a challenge test for a (node, script version). In
// the MVP it validates the script version is executable and grants a test
// window; the plugin runs the platform test params locally and this window is
// the pass marker. Returns the resulting test_expires_at.
func CreateCapabilityTest(c *gin.Context) {
	nodeId, scriptId, version, ok := parseCapabilityParams(c)
	if !ok {
		return
	}
	if version <= 0 {
		common.ApiErrorMsg(c, "version query param is required")
		return
	}
	if _, err := model.GetExecutableScriptVersion(scriptId, version); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	expiresAt := time.Now().Add(capabilityTestTTL).Unix()
	common.ApiSuccess(c, gin.H{
		"node_id":         nodeId,
		"script_id":       scriptId,
		"version":         version,
		"test_expires_at": expiresAt,
	})
}

type enableCapabilityRequest struct {
	Version       int    `json:"version"`
	PriceMicros   int64  `json:"price_micros"`
	DailyLimit    int    `json:"daily_limit"`
	WorkWindow    string `json:"work_window"`
	TestExpiresAt int64  `json:"test_expires_at"`
}

// EnableCapability lists a script version on a node with price and daily limit.
// Requires a valid test window (from CreateCapabilityTest) and an executable
// version. The initial balance defaults to 10 on first listing and is updated
// from actual execution results thereafter — the provider does not set it.
func EnableCapability(c *gin.Context) {
	nodeId, scriptId, _, ok := parseCapabilityParams(c)
	if !ok {
		return
	}
	var req enableCapabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Version <= 0 {
		common.ApiErrorMsg(c, "version is required")
		return
	}
	cap := &model.NodeCapability{
		NodeId:        nodeId,
		ScriptId:      scriptId,
		Version:       req.Version,
		UserId:        c.GetInt("id"),
		PriceMicros:   req.PriceMicros,
		DailyLimit:    req.DailyLimit,
		WorkWindow:    req.WorkWindow,
		TestExpiresAt: req.TestExpiresAt,
	}
	if err := model.EnableCapability(cap); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, cap)
}

// DisableCapability removes a provider capability from the market. The route
// name is retained for API compatibility; the record itself is deleted.
func DisableCapability(c *gin.Context) {
	nodeId, scriptId, version, ok := parseCapabilityParams(c)
	if !ok {
		return
	}
	if version <= 0 {
		common.ApiErrorMsg(c, "version query param is required")
		return
	}
	if err := model.RemoveCapability(c.GetInt("id"), nodeId, scriptId, version); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, nil)
}

// ListCapabilities returns a node's capabilities.
func ListCapabilities(c *gin.Context) {
	nodeId := c.Param("id")
	caps, err := model.ListNodeCapabilities(nodeId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, caps)
}

// ListMyNodes returns the authenticated user's nodes.
func ListMyNodes(c *gin.Context) {
	nodes, err := model.ListNodesByUser(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nodes)
}

// ListMyCapabilityStats returns per-(node, script, version) execution stats
// (executions, successes, revenue) across the caller's nodes.
func ListMyCapabilityStats(c *gin.Context) {
	stats, err := model.GetProviderCapabilityStats(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

// DeleteMyNode permanently removes an OFFLINE node and its capabilities.
func DeleteMyNode(c *gin.Context) {
	nodeId := c.Param("id")
	if err := model.DeleteOfflineNode(c.GetInt("id"), nodeId); err != nil {
		if errors.Is(err, model.ErrNodeNotFound) {
			common.ApiErrorMsg(c, "node not found")
			return
		}
		if errors.Is(err, model.ErrNodeStillOnline) {
			common.ApiErrorMsg(c, "node is still online; wait until it is offline")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
