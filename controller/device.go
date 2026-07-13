package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// CreateDeviceChallenge issues a signing challenge for the authenticated user's
// new device. The plugin signs the returned nonce with its Ed25519 device key.
func CreateDeviceChallenge(c *gin.Context) {
	userId := c.GetInt("id")
	ch, err := model.CreateDeviceChallenge(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"challenge_id": ch.Id,
		"nonce":        ch.Nonce,
		"expires_at":   ch.ExpiresAt,
	})
}

type activateDeviceRequest struct {
	ChallengeId string `json:"challenge_id"`
	PublicKey   string `json:"public_key"` // base64 Ed25519
	Signature   string `json:"signature"`  // base64 signature over challenge message
	Name        string `json:"name"`
}

// ActivateDevice verifies the signed challenge and returns the device session
// (short-lived access token + device-bound refresh token).
func ActivateDevice(c *gin.Context) {
	userId := c.GetInt("id")
	var req activateDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ChallengeId == "" || req.PublicKey == "" || req.Signature == "" {
		common.ApiErrorMsg(c, "challenge_id, public_key and signature are required")
		return
	}
	device, session, err := model.ActivateDevice(userId, req.ChallengeId, req.PublicKey, req.Signature, req.Name)
	if err != nil {
		if errors.Is(err, model.ErrChallengeInvalid) {
			common.ApiErrorMsg(c, "challenge invalid or expired")
			return
		}
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"device": device, "session": session})
}

type refreshDeviceRequest struct {
	DeviceId     string `json:"device_id"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshDeviceSession rotates a device's token pair using its refresh token.
func RefreshDeviceSession(c *gin.Context) {
	var req refreshDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	session, err := model.RefreshDeviceSession(req.DeviceId, req.RefreshToken)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, session)
}

// ListMyDevices returns the authenticated user's devices.
func ListMyDevices(c *gin.Context) {
	devices, err := model.ListUserDevices(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, devices)
}

// RevokeMyDevice revokes a device and suspends its nodes/capabilities.
func RevokeMyDevice(c *gin.Context) {
	deviceId := c.Param("deviceId")
	if deviceId == "" {
		common.ApiErrorMsg(c, "device id is required")
		return
	}
	if err := model.RevokeDevice(c.GetInt("id"), deviceId); err != nil {
		if errors.Is(err, model.ErrDeviceNotFound) {
			common.ApiErrorMsg(c, "device not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
