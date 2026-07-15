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
	// Balance is the script-reported target-site account balance, sent on the
	// provider receipt so it is persisted before settlement reads it. The
	// result_ready control frame carries it too, but that frame can arrive after
	// reconciliation; the receipt POST is what triggers settlement, so storing it
	// here closes the race. Only a positive value is stored.
	Balance int `json:"balance"`
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
	// Persist the provider-reported balance before reconciling so the settlement
	// layer can update the capability's remaining_quota in the same request that
	// settles the order (the result_ready frame may not have arrived yet).
	if req.Party == "provider" && req.Balance > 0 {
		_ = model.SetTaskAttemptBalance(req.TaskId, req.Attempt, req.Balance)
	}
	// Receipts can beat the control-channel result_ready frame by a few
	// milliseconds. Bring a running order to its reconciliation state so a
	// matching pair can settle deterministically regardless of arrival order.
	if o, _ := model.GetOrder(req.OrderId); o != nil && o.State == model.OrderRunning {
		_, _ = model.ApplyTransition(req.OrderId, model.OrderResultReady, nil)
		_, _ = model.ApplyTransition(req.OrderId, model.OrderVerifying, nil)
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
