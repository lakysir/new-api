package controller

import (
	"context"
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/payment"
	"github.com/QuantumNous/new-api/service/settlement"
	"github.com/gin-gonic/gin"
)

// paymentService builds the active payment service. The MVP uses the Fake
// adapter (simulated funds); Stage G swaps in a testnet then mainnet adapter
// behind KMS-held keys without changing callers.
func paymentService() *payment.Service {
	// One confirmation is enough for the Fake network.
	return payment.NewService(payment.NewFakeAdapter("fake", 100), settlement.Currency, 1)
}

// CreateDepositAddress returns a funding address for the authenticated user.
func CreateDepositAddress(c *gin.Context) {
	svc := payment.NewFakeAdapter("fake", 100)
	addr, err := svc.CreateDepositAddress(context.Background(), toAccountId(c.GetInt("id")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, addr)
}

type withdrawRequest struct {
	OwnerType    string `json:"owner_type"` // provider | author
	ToAddress    string `json:"to_address"`
	AmountMicros int64  `json:"amount_micros"`
}

// RequestWithdrawal submits a withdrawal of the caller's payable balance. It
// enforces the dynamic minimum (fee vs amount) and available payable balance,
// and shows the fee quote in the response before the funds leave.
func RequestWithdrawal(c *gin.Context) {
	var req withdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	ownerType := req.OwnerType
	if ownerType != model.OwnerProvider && ownerType != model.OwnerAuthor {
		common.ApiErrorMsg(c, "owner_type must be provider or author")
		return
	}
	if req.ToAddress == "" {
		common.ApiErrorMsg(c, "to_address is required")
		return
	}
	wd, fee, err := paymentService().RequestWithdrawal(context.Background(), payment.WithdrawInput{
		OwnerType: ownerType, OwnerId: c.GetInt("id"), ToAddress: req.ToAddress, AmountMicros: req.AmountMicros,
	})
	if err != nil {
		if errors.Is(err, payment.ErrBelowMinimum) {
			common.ApiErrorMsg(c, "amount below dynamic minimum for current network fee")
			return
		}
		if errors.Is(err, payment.ErrInsufficientWithdrawable) {
			common.ApiErrorMsg(c, "insufficient withdrawable balance")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"withdrawal": wd,
		"fee":        fee,
	})
}

// EstimateWithdrawalFee returns the current network fee estimate for an amount.
func EstimateWithdrawalFee(c *gin.Context) {
	var req withdrawRequest
	_ = c.ShouldBindJSON(&req)
	fee, err := payment.NewFakeAdapter("fake", 100).EstimateWithdrawalFee(context.Background(), req.AmountMicros)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, fee)
}

func toAccountId(userId int) string {
	return strconv.Itoa(userId)
}
