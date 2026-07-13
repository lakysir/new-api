// Double-entry ledger. Every transaction posts >=2 entries whose debits equal
// credits; account balances are maintained as balance = Σcredits - Σdebits
// (all user-facing accounts are credit-normal, so their balances stay
// non-negative). Entries are immutable — corrections post a reversal
// transaction (architecture §14). All money is integer micro-USD.
package model

import (
	"errors"
)

// Account owner types.
const (
	OwnerClient   = "client"
	OwnerProvider = "provider"
	OwnerAuthor   = "author"
	OwnerPlatform = "platform"
	OwnerExternal = "external" // funding source (deposits enter from here)
)

// Account kinds. A single owner can have several kinds (available, reserved,
// payable, revenue).
const (
	KindAvailable = "available"
	KindReserved  = "reserved"
	KindPayable   = "payable"
	KindRevenue   = "revenue"
	KindReserve   = "reserve" // platform-held cost/risk reserves
	KindSource    = "source"  // external funding source
)

// Ledger transaction types.
const (
	TxDeposit  = "deposit"
	TxReserve  = "reserve"
	TxSettle   = "settle"
	TxRefund   = "refund"
	TxRelease  = "release"
	TxReversal = "reversal"
)

// Entry directions.
const (
	DirDebit  = "debit"
	DirCredit = "credit"
)

// Platform owner id 0 is the singleton platform.
const PlatformOwnerId = 0

var (
	// ErrUnbalanced is returned when a transaction's debits != credits.
	ErrUnbalanced = errors.New("ledger transaction is not balanced")
	// ErrNoEntries is returned when a transaction has fewer than two entries.
	ErrNoEntries = errors.New("ledger transaction needs at least two entries")
	// ErrNonPositiveAmount is returned for a zero/negative entry amount.
	ErrNonPositiveAmount = errors.New("ledger entry amount must be positive")
	// ErrInsufficientBalance is returned when a debit would overdraw a real
	// (non-external) account below zero.
	ErrInsufficientBalance = errors.New("insufficient balance")
)

// LedgerAccount is a balance bucket for one (owner, kind, currency).
type LedgerAccount struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	OwnerType     string `json:"owner_type" gorm:"type:varchar(16);uniqueIndex:idx_account;not null"`
	OwnerId       int    `json:"owner_id" gorm:"uniqueIndex:idx_account;not null"`
	Kind          string `json:"kind" gorm:"type:varchar(16);uniqueIndex:idx_account;not null"`
	Currency      string `json:"currency" gorm:"type:varchar(8);uniqueIndex:idx_account;not null"`
	BalanceMicros int64  `json:"balance_micros" gorm:"default:0"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (LedgerAccount) TableName() string { return "ledger_accounts" }

// LedgerTransaction groups a balanced set of entries. IdempotencyKey is unique
// so a retried operation posts exactly once (architecture §8.3 account key).
type LedgerTransaction struct {
	Id             string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	IdempotencyKey string `json:"idempotency_key" gorm:"type:varchar(96);uniqueIndex;not null"`
	Type           string `json:"type" gorm:"type:varchar(16);index"`
	ReferenceId    string `json:"reference_id" gorm:"type:varchar(64);index"`
	Status         string `json:"status" gorm:"type:varchar(16);default:posted"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (LedgerTransaction) TableName() string { return "ledger_transactions" }

// LedgerEntry is one immutable leg of a transaction.
type LedgerEntry struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	TransactionId string `json:"transaction_id" gorm:"type:varchar(64);index;not null"`
	AccountId     int    `json:"account_id" gorm:"index;not null"`
	Direction     string `json:"direction" gorm:"type:varchar(8);not null"`
	AmountMicros  int64  `json:"amount_micros" gorm:"not null"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (LedgerEntry) TableName() string { return "ledger_entries" }

// EntryInput describes one leg to post. Account is addressed by its natural key
// so callers don't need account ids.
type EntryInput struct {
	OwnerType string
	OwnerId   int
	Kind      string
	Currency  string
	Direction string
	Amount    int64
}

// signedDelta returns the balance delta a direction applies under the
// credit-normal convention (balance = Σcredits - Σdebits).
func signedDelta(direction string, amount int64) int64 {
	if direction == DirCredit {
		return amount
	}
	return -amount
}
