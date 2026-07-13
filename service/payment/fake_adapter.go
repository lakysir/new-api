package payment

import (
	"context"
	"fmt"
	"sync"
)

// FakeAdapter is an in-memory Adapter for tests and the simulated-funds path.
// It deterministically generates addresses, lets tests inject deposits and
// records submitted withdrawals. It is concurrency-safe.
type FakeAdapter struct {
	network string

	mu          sync.Mutex
	deposits    []DepositEvent
	scanned     int // cursor position into deposits
	withdrawals map[string]*PaymentTransaction
	feeMicros   int64
	nextId      int
}

// NewFakeAdapter builds a Fake adapter for the given network name with a fixed
// per-withdrawal fee estimate.
func NewFakeAdapter(network string, feeMicros int64) *FakeAdapter {
	return &FakeAdapter{
		network:     network,
		withdrawals: make(map[string]*PaymentTransaction),
		feeMicros:   feeMicros,
	}
}

// Network returns the adapter's network name.
func (f *FakeAdapter) Network() string { return f.network }

// CreateDepositAddress returns a deterministic address for an account.
func (f *FakeAdapter) CreateDepositAddress(_ context.Context, accountId string) (*DepositAddress, error) {
	return &DepositAddress{
		Network:   f.network,
		Address:   fmt.Sprintf("fake-%s-%s", f.network, accountId),
		AccountId: accountId,
	}, nil
}

// InjectDeposit is a test helper to enqueue an inbound transfer.
func (f *FakeAdapter) InjectDeposit(e DepositEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e.Network == "" {
		e.Network = f.network
	}
	f.deposits = append(f.deposits, e)
}

// ScanDeposits returns deposits observed since the cursor and the new cursor.
// The cursor is the count of already-consumed events, so a re-scan with the
// same cursor is idempotent (no duplicate credit).
func (f *FakeAdapter) ScanDeposits(_ context.Context, cursor string) ([]DepositEvent, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	start := 0
	fmt.Sscanf(cursor, "%d", &start)
	if start < 0 || start > len(f.deposits) {
		start = 0
	}
	batch := append([]DepositEvent(nil), f.deposits[start:]...)
	newCursor := fmt.Sprintf("%d", len(f.deposits))
	return batch, newCursor, nil
}

// EstimateWithdrawalFee returns the configured fee estimate.
func (f *FakeAdapter) EstimateWithdrawalFee(_ context.Context, _ int64) (*FeeQuote, error) {
	return &FeeQuote{Network: f.network, FeeMicros: f.feeMicros, EstimatedSec: 60}, nil
}

// SubmitWithdrawal records a pending withdrawal and returns its external id.
func (f *FakeAdapter) SubmitWithdrawal(_ context.Context, req WithdrawalRequest) (*Withdrawal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextId++
	extId := fmt.Sprintf("fakewd-%s-%d", f.network, f.nextId)
	f.withdrawals[extId] = &PaymentTransaction{
		Network: f.network, ExternalId: extId,
		AmountMicros: req.AmountMicros, Status: StatusPending,
	}
	return &Withdrawal{ExternalId: extId, Status: StatusPending}, nil
}

// ConfirmWithdrawal is a test helper to mark a withdrawal confirmed.
func (f *FakeAdapter) ConfirmWithdrawal(externalId string, confirmations int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if tx, ok := f.withdrawals[externalId]; ok {
		tx.Status = StatusConfirmed
		tx.Confirmations = confirmations
	}
}

// GetTransaction returns a recorded transaction by external id.
func (f *FakeAdapter) GetTransaction(_ context.Context, externalId string) (*PaymentTransaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if tx, ok := f.withdrawals[externalId]; ok {
		cp := *tx
		return &cp, nil
	}
	return nil, ErrUnknownTransaction
}
