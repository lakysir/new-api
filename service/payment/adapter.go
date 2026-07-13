// Package payment defines the network-agnostic Payment Adapter interface and a
// Fake in-memory implementation for tests and simulated funds. Real networks
// (USDT-TRC20 etc.) implement the same interface behind KMS-held keys; the
// implementation order is Fake -> testnet -> small mainnet canary
// (architecture §22.7). No real private key ever lives in the app DB or .env.
package payment

import (
	"context"
	"errors"
)

// DepositAddress is a per-account funding address on a payment network.
type DepositAddress struct {
	Network   string `json:"network"`
	Address   string `json:"address"`
	AccountId string `json:"account_id"`
}

// DepositEvent is an observed inbound transfer.
type DepositEvent struct {
	Network       string `json:"network"`
	ExternalId    string `json:"external_id"`
	AccountId     string `json:"account_id"`
	AmountMicros  int64  `json:"amount_micros"`
	Confirmations int    `json:"confirmations"`
}

// FeeQuote is an estimated withdrawal network fee. Fees are estimated live
// (Energy/Bandwidth for TRON), never hardcoded (architecture §14.5).
type FeeQuote struct {
	Network      string `json:"network"`
	FeeMicros    int64  `json:"fee_micros"`
	EstimatedSec int    `json:"estimated_confirmation_seconds"`
}

// WithdrawalRequest asks the adapter to send funds out.
type WithdrawalRequest struct {
	AccountId    string `json:"account_id"`
	ToAddress    string `json:"to_address"`
	AmountMicros int64  `json:"amount_micros"`
}

// Withdrawal is the result of submitting a withdrawal.
type Withdrawal struct {
	ExternalId string `json:"external_id"`
	Status     string `json:"status"`
}

// PaymentTransaction is a normalized view of an on-chain transaction.
type PaymentTransaction struct {
	Network       string `json:"network"`
	ExternalId    string `json:"external_id"`
	AmountMicros  int64  `json:"amount_micros"`
	Confirmations int    `json:"confirmations"`
	Status        string `json:"status"`
}

// Transaction / withdrawal statuses.
const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusFailed    = "failed"
)

// ErrUnknownTransaction is returned when an external id is not found.
var ErrUnknownTransaction = errors.New("unknown payment transaction")

// Adapter is the network-agnostic payment port (architecture §22.7).
type Adapter interface {
	Network() string
	CreateDepositAddress(ctx context.Context, accountId string) (*DepositAddress, error)
	ScanDeposits(ctx context.Context, cursor string) ([]DepositEvent, string, error)
	EstimateWithdrawalFee(ctx context.Context, amountMicros int64) (*FeeQuote, error)
	SubmitWithdrawal(ctx context.Context, req WithdrawalRequest) (*Withdrawal, error)
	GetTransaction(ctx context.Context, externalId string) (*PaymentTransaction, error)
}
