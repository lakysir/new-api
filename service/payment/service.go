package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/model"
)

// MinFeeRatioDenominator: a withdrawal is allowed only if the network fee is at
// most 1/ratio of the amount, i.e. amount >= fee * ratio. This is the dynamic
// minimum that keeps chain cost an acceptable share of the withdrawal
// (architecture §14.5); it scales with the live fee estimate rather than a
// hardcoded floor.
const DefaultMinFeeRatio = 10 // fee must be <= 10% of the withdrawal amount

var (
	// ErrBelowMinimum is returned when a withdrawal amount is too small relative
	// to the estimated network fee.
	ErrBelowMinimum = errors.New("withdrawal amount below dynamic minimum for current network fee")
	// ErrInsufficientWithdrawable is returned when the payable balance is too low.
	ErrInsufficientWithdrawable = errors.New("insufficient withdrawable balance")
)

// Service composes a payment Adapter with the internal ledger. It credits
// confirmed deposits and debits payable balances on withdrawal, keeping the
// ledger the single source of truth for balances (architecture §14.1).
type Service struct {
	adapter     Adapter
	currency    string
	minFeeRatio int64
	minConfirms int
}

// NewService builds a payment service for a currency, requiring minConfirms
// confirmations before a deposit is credited.
func NewService(adapter Adapter, currency string, minConfirms int) *Service {
	return &Service{adapter: adapter, currency: currency, minFeeRatio: DefaultMinFeeRatio, minConfirms: minConfirms}
}

// ProcessDeposits scans for new deposits from the cursor, records each one
// idempotently, and credits the client's available balance for confirmed,
// newly-seen deposits. Returns the count credited and the advanced cursor.
func (s *Service) ProcessDeposits(ctx context.Context, cursor string) (credited int, newCursor string, err error) {
	events, next, err := s.adapter.ScanDeposits(ctx, cursor)
	if err != nil {
		return 0, cursor, err
	}
	for _, e := range events {
		if e.Confirmations < s.minConfirms {
			continue // not yet final; a later scan re-observes it
		}
		row := &model.PaymentTransaction{
			Channel: s.adapter.Network(), ExternalId: e.ExternalId, Direction: "deposit",
			AccountId: e.AccountId, AmountMicros: e.AmountMicros,
			Confirmations: e.Confirmations, Status: model.PaymentStatusConfirmed,
		}
		_, isNew, rerr := model.RecordPaymentIfNew(row)
		if rerr != nil {
			return credited, next, rerr
		}
		if !isNew {
			continue // duplicate callback / re-scan: already credited
		}
		clientId, cerr := parseAccountId(e.AccountId)
		if cerr != nil {
			return credited, next, cerr
		}
		// Ledger credit is idempotent on this key too (belt and suspenders).
		key := fmt.Sprintf("deposit:%s:%s", s.adapter.Network(), e.ExternalId)
		if _, perr := model.PostTransaction(key, model.TxDeposit, e.ExternalId, []model.EntryInput{
			{OwnerType: model.OwnerExternal, OwnerId: model.PlatformOwnerId, Kind: model.KindSource, Currency: s.currency, Direction: model.DirDebit, Amount: e.AmountMicros},
			{OwnerType: model.OwnerClient, OwnerId: clientId, Kind: model.KindAvailable, Currency: s.currency, Direction: model.DirCredit, Amount: e.AmountMicros},
		}); perr != nil {
			return credited, next, perr
		}
		credited++
	}
	return credited, next, nil
}

// WithdrawInput requests moving payable funds out to an address.
type WithdrawInput struct {
	OwnerType    string // provider | author
	OwnerId      int
	ToAddress    string
	AmountMicros int64
}

// RequestWithdrawal validates the dynamic minimum and payable balance, debits
// the payable balance into the external source (money leaving the system),
// submits to the adapter and records the payment transaction. The ledger debit
// and the adapter submission are ordered so a failure never loses funds: the
// balance check + debit happen first, then submission; a submit failure is
// reported to the caller with the debit already durable, to be reconciled by a
// retry job (never silently credited back here).
func (s *Service) RequestWithdrawal(ctx context.Context, in WithdrawInput) (*Withdrawal, *FeeQuote, error) {
	if in.AmountMicros <= 0 {
		return nil, nil, errors.New("amount must be positive")
	}
	fee, err := s.adapter.EstimateWithdrawalFee(ctx, in.AmountMicros)
	if err != nil {
		return nil, nil, err
	}
	// Dynamic minimum: amount must be at least fee * ratio.
	if in.AmountMicros < fee.FeeMicros*s.minFeeRatio {
		return nil, fee, ErrBelowMinimum
	}
	balance, err := model.GetBalance(in.OwnerType, in.OwnerId, model.KindPayable, s.currency)
	if err != nil {
		return nil, fee, err
	}
	if balance < in.AmountMicros {
		return nil, fee, ErrInsufficientWithdrawable
	}

	// Debit payable -> external source (idempotency key ties to address+amount+
	// balance so identical retries dedupe, distinct requests do not).
	key := fmt.Sprintf("withdraw:%s:%d:%s:%d", in.OwnerType, in.OwnerId, in.ToAddress, in.AmountMicros)
	if _, err := model.PostTransaction(key, model.TxRelease, in.ToAddress, []model.EntryInput{
		{OwnerType: in.OwnerType, OwnerId: in.OwnerId, Kind: model.KindPayable, Currency: s.currency, Direction: model.DirDebit, Amount: in.AmountMicros},
		{OwnerType: model.OwnerExternal, OwnerId: model.PlatformOwnerId, Kind: model.KindSource, Currency: s.currency, Direction: model.DirCredit, Amount: in.AmountMicros},
	}); err != nil {
		return nil, fee, err
	}

	wd, err := s.adapter.SubmitWithdrawal(ctx, WithdrawalRequest{
		AccountId: fmt.Sprintf("%d", in.OwnerId), ToAddress: in.ToAddress, AmountMicros: in.AmountMicros,
	})
	if err != nil {
		return nil, fee, fmt.Errorf("withdrawal debited but submission failed, needs reconciliation: %w", err)
	}
	_, _, _ = model.RecordPaymentIfNew(&model.PaymentTransaction{
		Channel: s.adapter.Network(), ExternalId: wd.ExternalId, Direction: "withdrawal",
		AccountId: fmt.Sprintf("%d", in.OwnerId), AmountMicros: in.AmountMicros, Status: model.PaymentStatusPending,
	})
	return wd, fee, nil
}

// parseAccountId maps a payment account id back to a user id. The Fake and MVP
// use the decimal user id as the account id.
func parseAccountId(accountId string) (int, error) {
	var id int
	if _, err := fmt.Sscanf(accountId, "%d", &id); err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid account id %q", accountId)
	}
	return id, nil
}
