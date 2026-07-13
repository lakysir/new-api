package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/settlement"
	"github.com/gin-gonic/gin"
)

// GetMyLedgerBalances returns the authenticated user's ledger balances across
// the account kinds relevant to them (client available/reserved, payables).
func GetMyLedgerBalances(c *gin.Context) {
	userId := c.GetInt("id")
	cur := settlement.Currency
	available, _ := model.GetBalance(model.OwnerClient, userId, model.KindAvailable, cur)
	reserved, _ := model.GetBalance(model.OwnerClient, userId, model.KindReserved, cur)
	providerPayable, _ := model.GetBalance(model.OwnerProvider, userId, model.KindPayable, cur)
	authorPayable, _ := model.GetBalance(model.OwnerAuthor, userId, model.KindPayable, cur)
	common.ApiSuccess(c, gin.H{
		"currency":         cur,
		"client_available": available,
		"client_reserved":  reserved,
		"provider_payable": providerPayable,
		"author_payable":   authorPayable,
	})
}

type simulatedDepositRequest struct {
	AmountMicros int64  `json:"amount_micros"`
	Reference    string `json:"reference"`
}

// SimulatedDeposit credits the caller's available balance with simulated funds.
// This is the Stage F test-money path (USD_TEST) — real USDT deposits arrive
// via the Payment Adapter in Stage G. Gated behind auth; amount must be > 0.
func SimulatedDeposit(c *gin.Context) {
	var req simulatedDepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.AmountMicros <= 0 {
		common.ApiErrorMsg(c, "amount_micros must be positive")
		return
	}
	if req.Reference == "" {
		common.ApiErrorMsg(c, "reference is required for idempotency")
		return
	}
	tx, err := settlement.Deposit(c.GetInt("id"), req.AmountMicros, req.Reference)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"transaction_id": tx.Id, "type": tx.Type})
}
