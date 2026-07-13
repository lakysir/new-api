// Package settlement composes order state transitions with double-entry ledger
// postings: deposit, fund reservation, success split, refund and release. Each
// operation is idempotent via a derived ledger idempotency key so retries and
// duplicate events never double-post (architecture §14).
package settlement

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
)

// Currency for the MVP simulated money (no real USDT until Stage G).
const Currency = "USD_TEST"

// Deposit credits a client's available balance from the external funding source.
func Deposit(clientId int, amountMicros int64, reference string) (*model.LedgerTransaction, error) {
	key := fmt.Sprintf("deposit:%s", reference)
	return model.PostTransaction(key, model.TxDeposit, reference, []model.EntryInput{
		{OwnerType: model.OwnerExternal, OwnerId: model.PlatformOwnerId, Kind: model.KindSource, Currency: Currency, Direction: model.DirDebit, Amount: amountMicros},
		{OwnerType: model.OwnerClient, OwnerId: clientId, Kind: model.KindAvailable, Currency: Currency, Direction: model.DirCredit, Amount: amountMicros},
	})
}

// ReserveFunds moves the order's max amount from the client's available balance
// into a reserved bucket and advances the order to FUNDS_RESERVED. Insufficient
// balance fails without changing order state.
func ReserveFunds(orderId string) (*model.Order, error) {
	o, err := model.GetOrder(orderId)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("reserve:%s", orderId)
	if _, err := model.PostTransaction(key, model.TxReserve, orderId, []model.EntryInput{
		{OwnerType: model.OwnerClient, OwnerId: o.ClientId, Kind: model.KindAvailable, Currency: Currency, Direction: model.DirDebit, Amount: o.MaxAmountMicros},
		{OwnerType: model.OwnerClient, OwnerId: o.ClientId, Kind: model.KindReserved, Currency: Currency, Direction: model.DirCredit, Amount: o.MaxAmountMicros},
	}); err != nil {
		return nil, err
	}
	return model.ApplyTransition(orderId, model.OrderFundsReserved, nil)
}

// SettleParticipants is the resolved payout split at settlement time. Amounts
// are micro-USD and must sum to <= the reserved max (the remainder is released).
type SettleParticipants struct {
	ProviderId           int
	AuthorId             int
	ProviderMicros       int64
	AuthorMicros         int64
	PlatformMicros       int64
	NetworkReserveMicros int64
}

// Settle consumes the client's reserved funds, pays provider/author/platform,
// releases any unused remainder back to the client's available balance, and
// advances the order to SETTLED. All postings share the order as reference and
// are idempotent.
func Settle(orderId string, p SettleParticipants) (*model.Order, error) {
	o, err := model.GetOrder(orderId)
	if err != nil {
		return nil, err
	}
	// Idempotent: an already-settled order returns unchanged (the ledger key
	// also dedupes the posting, so no double payout can occur either way).
	if o.State == model.OrderSettled {
		return o, nil
	}
	payout := p.ProviderMicros + p.AuthorMicros + p.PlatformMicros + p.NetworkReserveMicros
	if payout > o.MaxAmountMicros {
		return nil, fmt.Errorf("settlement payout %d exceeds reserved %d", payout, o.MaxAmountMicros)
	}
	remainder := o.MaxAmountMicros - payout

	entries := []model.EntryInput{
		// Debit the whole reserved amount so it nets to zero.
		{OwnerType: model.OwnerClient, OwnerId: o.ClientId, Kind: model.KindReserved, Currency: Currency, Direction: model.DirDebit, Amount: o.MaxAmountMicros},
	}
	if p.ProviderMicros > 0 {
		entries = append(entries, model.EntryInput{OwnerType: model.OwnerProvider, OwnerId: p.ProviderId, Kind: model.KindPayable, Currency: Currency, Direction: model.DirCredit, Amount: p.ProviderMicros})
	}
	if p.AuthorMicros > 0 {
		entries = append(entries, model.EntryInput{OwnerType: model.OwnerAuthor, OwnerId: p.AuthorId, Kind: model.KindPayable, Currency: Currency, Direction: model.DirCredit, Amount: p.AuthorMicros})
	}
	if p.PlatformMicros > 0 {
		entries = append(entries, model.EntryInput{OwnerType: model.OwnerPlatform, OwnerId: model.PlatformOwnerId, Kind: model.KindRevenue, Currency: Currency, Direction: model.DirCredit, Amount: p.PlatformMicros})
	}
	if p.NetworkReserveMicros > 0 {
		entries = append(entries, model.EntryInput{OwnerType: model.OwnerPlatform, OwnerId: model.PlatformOwnerId, Kind: model.KindReserve, Currency: Currency, Direction: model.DirCredit, Amount: p.NetworkReserveMicros})
	}
	if remainder > 0 {
		entries = append(entries, model.EntryInput{OwnerType: model.OwnerClient, OwnerId: o.ClientId, Kind: model.KindAvailable, Currency: Currency, Direction: model.DirCredit, Amount: remainder})
	}

	key := fmt.Sprintf("settle:%s", orderId)
	if _, err := model.PostTransaction(key, model.TxSettle, orderId, entries); err != nil {
		return nil, err
	}
	return model.ApplyTransition(orderId, model.OrderSettled, map[string]any{"final_amount_micros": payout})
}

// Refund returns the full reserved amount to the client's available balance and
// advances the order to REFUNDED. Used for pre-execution failures and the MVP
// execution-failure policy (full refund).
func Refund(orderId string) (*model.Order, error) {
	o, err := model.GetOrder(orderId)
	if err != nil {
		return nil, err
	}
	// Idempotent: an already-refunded order returns unchanged.
	if o.State == model.OrderRefunded {
		return o, nil
	}
	key := fmt.Sprintf("refund:%s", orderId)
	if _, err := model.PostTransaction(key, model.TxRefund, orderId, []model.EntryInput{
		{OwnerType: model.OwnerClient, OwnerId: o.ClientId, Kind: model.KindReserved, Currency: Currency, Direction: model.DirDebit, Amount: o.MaxAmountMicros},
		{OwnerType: model.OwnerClient, OwnerId: o.ClientId, Kind: model.KindAvailable, Currency: Currency, Direction: model.DirCredit, Amount: o.MaxAmountMicros},
	}); err != nil {
		return nil, err
	}
	return model.ApplyTransition(orderId, model.OrderRefunded, nil)
}
