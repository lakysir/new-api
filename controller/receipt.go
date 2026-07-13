package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/settlement"
	"github.com/gin-gonic/gin"
)

type submitReceiptRequest struct {
	TaskId       string `json:"task_id"`
	Attempt      int    `json:"attempt"`
	Party        string `json:"party"` // "client" or "provider"
	OrderId      string `json:"order_id"`
	InputHash    string `json:"input_hash"`
	ResultHash   string `json:"result_hash"`
	PayloadHash  string `json:"payload_hash"`
	Signature    string `json:"signature"`
	SignerDevice string `json:"signer_device"`
}

// SubmitReceipt stores a signed party receipt for a task attempt, then attempts
// reconciliation. When both parties' receipts are present, matching receipts
// settle the order and mismatching receipts route it to dispute. Only hashes
// and the signature are stored — never plaintext.
func SubmitReceipt(c *gin.Context) {
	var req submitReceiptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.TaskId == "" || req.OrderId == "" || (req.Party != "client" && req.Party != "provider") {
		common.ApiErrorMsg(c, "task_id, order_id and a valid party are required")
		return
	}
	r := &model.Receipt{
		TaskId: req.TaskId, Attempt: req.Attempt, Party: req.Party, OrderId: req.OrderId,
		InputHash: req.InputHash, ResultHash: req.ResultHash, PayloadHash: req.PayloadHash,
		Signature: req.Signature, SignerDevice: req.SignerDevice,
	}
	if err := model.SaveReceipt(r); err != nil {
		common.ApiError(c, err)
		return
	}

	// Try to reconcile; incomplete is a normal "waiting for the other party".
	result, err := settlement.ReconcileAndSettle(req.OrderId, req.TaskId, req.Attempt)
	if err != nil {
		if errors.Is(err, settlement.ErrReceiptsIncomplete) {
			common.ApiSuccess(c, gin.H{"stored": true, "reconciled": false})
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"stored":     true,
		"reconciled": true,
		"matched":    result.Matched,
		"state":      result.Order.State,
		"reason":     result.Reason,
	})
}
