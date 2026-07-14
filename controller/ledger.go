package controller

import (
	"fmt"
	"math"
	"time"

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

// periodStarts returns the unix timestamps for the start of today, this week
// (Monday) and this month in the server's local time. Earnings pages sum credits
// posted at or after each boundary to show day/week/month income.
func periodStarts(now time.Time) (day, week, month int64) {
	loc := now.Location()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	// Week starts Monday. Go's Sunday=0..Saturday=6; shift so Monday=0.
	weekday := (int(dayStart.Weekday()) + 6) % 7
	weekStart := dayStart.AddDate(0, 0, -weekday)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	return dayStart.Unix(), weekStart.Unix(), monthStart.Unix()
}

// earningsFor builds the day/week/month/total earnings and current balance for
// one ledger account (owner + kind). All amounts are micro-USD.
func earningsFor(ownerType string, ownerId int, kind string) (gin.H, error) {
	cur := settlement.Currency
	now := time.Now()
	dayStart, weekStart, monthStart := periodStarts(now)
	balance, err := model.GetBalance(ownerType, ownerId, kind, cur)
	if err != nil {
		return nil, err
	}
	day, err := model.SumCreditsSince(ownerType, ownerId, kind, cur, dayStart)
	if err != nil {
		return nil, err
	}
	week, err := model.SumCreditsSince(ownerType, ownerId, kind, cur, weekStart)
	if err != nil {
		return nil, err
	}
	month, err := model.SumCreditsSince(ownerType, ownerId, kind, cur, monthStart)
	if err != nil {
		return nil, err
	}
	total, err := model.SumCreditsSince(ownerType, ownerId, kind, cur, 0)
	if err != nil {
		return nil, err
	}
	return gin.H{
		"currency":       cur,
		"balance_micros": balance,
		"day_micros":     day,
		"week_micros":    week,
		"month_micros":   month,
		"total_micros":   total,
	}, nil
}

// GetMyEarnings returns the caller's earnings summary for a payable role
// (?role=provider|author, default provider): current payable balance plus
// day/week/month/lifetime gross credits to that account.
func GetMyEarnings(c *gin.Context) {
	userId := c.GetInt("id")
	ownerType := model.OwnerProvider
	if c.Query("role") == "author" {
		ownerType = model.OwnerAuthor
	}
	result, err := earningsFor(ownerType, userId, model.KindPayable)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// GetPlatformEarnings returns the platform's revenue summary (admin only): the
// running revenue balance plus day/week/month/lifetime service-fee income.
func GetPlatformEarnings(c *gin.Context) {
	result, err := earningsFor(model.OwnerPlatform, model.PlatformOwnerId, model.KindRevenue)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

type rechargeAvailableRequest struct {
	AmountMicros int64 `json:"amount_micros"`
}

// microsToQuota converts a micro-USD amount into the equivalent wallet quota
// (tokens). The marketplace available-balance ledger and the main wallet both
// value 1 USD identically (a 1:1 recharge): 1 USD = 1,000,000 micros and
// QuotaPerUnit tokens. So quota = amountMicros / 1,000,000 * QuotaPerUnit.
func microsToQuota(amountMicros int64) int {
	return int(math.Round(float64(amountMicros) / 1_000_000 * common.QuotaPerUnit))
}

// RechargeAvailable funds the caller's marketplace available balance by
// deducting the equivalent amount from their main wallet quota (the same
// balance shown on /wallet). This is a real transfer — not simulated: the
// wallet quota is debited and the available-balance ledger is credited 1:1 in
// USD. On any ledger failure the quota is refunded so the two never diverge.
func RechargeAvailable(c *gin.Context) {
	var req rechargeAvailableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.AmountMicros <= 0 {
		common.ApiErrorMsg(c, "amount_micros must be positive")
		return
	}
	quota := microsToQuota(req.AmountMicros)
	if quota <= 0 {
		common.ApiErrorMsg(c, "amount too small to recharge")
		return
	}

	userId := c.GetInt("id")
	balance, err := model.GetUserQuota(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if balance < quota {
		common.ApiErrorMsg(c, "insufficient wallet balance")
		return
	}

	// Debit the wallet quota first, then credit the available balance. If the
	// ledger post fails we return the quota so no funds are lost.
	if err := model.DecreaseUserQuota(userId, quota, true); err != nil {
		common.ApiError(c, err)
		return
	}
	reference := fmt.Sprintf("wallet:%d:%d", userId, time.Now().UnixNano())
	tx, err := settlement.Deposit(userId, req.AmountMicros, reference)
	if err != nil {
		if refundErr := model.IncreaseUserQuota(userId, quota, true); refundErr != nil {
			common.SysError(fmt.Sprintf("recharge deposit failed and quota refund failed for user %d: deposit=%v refund=%v", userId, err, refundErr))
		}
		common.ApiError(c, err)
		return
	}
	model.RecordLog(userId, model.LogTypeManage, fmt.Sprintf("充值可用余额 %d micros，扣除钱包额度 %d", req.AmountMicros, quota))
	common.ApiSuccess(c, gin.H{"transaction_id": tx.Id, "type": tx.Type, "quota_deducted": quota})
}
