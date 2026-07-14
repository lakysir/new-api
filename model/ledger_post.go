package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// getOrCreateAccountTx resolves an account by natural key within a transaction,
// creating it at zero balance if absent.
func getOrCreateAccountTx(tx *gorm.DB, ownerType string, ownerId int, kind, currency string) (*LedgerAccount, error) {
	var acc LedgerAccount
	err := tx.Where("owner_type = ? AND owner_id = ? AND kind = ? AND currency = ?",
		ownerType, ownerId, kind, currency).First(&acc).Error
	if err == nil {
		return &acc, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	acc = LedgerAccount{OwnerType: ownerType, OwnerId: ownerId, Kind: kind, Currency: currency}
	if err := tx.Create(&acc).Error; err != nil {
		// Lost a create race: re-read the winner.
		if isUniqueConstraintErr(err) {
			if rerr := tx.Where("owner_type = ? AND owner_id = ? AND kind = ? AND currency = ?",
				ownerType, ownerId, kind, currency).First(&acc).Error; rerr == nil {
				return &acc, nil
			}
		}
		return nil, err
	}
	return &acc, nil
}

// PostTransaction posts a balanced set of entries atomically and idempotently.
// If idempotencyKey already exists, the existing transaction is returned and no
// new entries are posted. Debits must equal credits and every amount must be
// positive; a debit that would drive a real (non-external) account negative is
// rejected with ErrInsufficientBalance.
func PostTransaction(idempotencyKey, txType, referenceId string, entries []EntryInput) (*LedgerTransaction, error) {
	if len(entries) < 2 {
		return nil, ErrNoEntries
	}
	var debits, credits int64
	for _, e := range entries {
		if e.Amount <= 0 {
			return nil, ErrNonPositiveAmount
		}
		if e.Direction == DirDebit {
			debits += e.Amount
		} else if e.Direction == DirCredit {
			credits += e.Amount
		} else {
			return nil, errors.New("invalid entry direction")
		}
	}
	if debits != credits {
		return nil, ErrUnbalanced
	}

	// Idempotency fast-path outside the transaction.
	if existing, err := findLedgerTx(idempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	var result *LedgerTransaction
	err := DB.Transaction(func(tx *gorm.DB) error {
		ltx := &LedgerTransaction{
			Id:             "ltx_" + common.GetUUID(),
			IdempotencyKey: idempotencyKey,
			Type:           txType,
			ReferenceId:    referenceId,
			Status:         "posted",
		}
		if err := tx.Create(ltx).Error; err != nil {
			return err
		}
		for _, e := range entries {
			currency := e.Currency
			if currency == "" {
				currency = "USD"
			}
			acc, err := getOrCreateAccountTx(tx, e.OwnerType, e.OwnerId, e.Kind, currency)
			if err != nil {
				return err
			}
			newBalance := acc.BalanceMicros + signedDelta(e.Direction, e.Amount)
			// Real accounts must not go negative; the external source may.
			if newBalance < 0 && e.OwnerType != OwnerExternal {
				return ErrInsufficientBalance
			}
			if err := tx.Create(&LedgerEntry{
				TransactionId: ltx.Id, AccountId: acc.Id,
				Direction: e.Direction, AmountMicros: e.Amount,
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&LedgerAccount{}).Where("id = ?", acc.Id).
				Update("balance_micros", newBalance).Error; err != nil {
				return err
			}
		}
		result = ltx
		return nil
	})
	if err != nil {
		// Idempotency race: another poster won on the unique key.
		if isUniqueConstraintErr(err) {
			if existing, ferr := findLedgerTx(idempotencyKey); ferr == nil && existing != nil {
				return existing, nil
			}
		}
		return nil, err
	}
	return result, nil
}

func findLedgerTx(idempotencyKey string) (*LedgerTransaction, error) {
	var ltx LedgerTransaction
	err := DB.Where("idempotency_key = ?", idempotencyKey).First(&ltx).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ltx, nil
}

// GetBalance returns an account's current balance (0 if the account does not
// exist yet).
func GetBalance(ownerType string, ownerId int, kind, currency string) (int64, error) {
	var acc LedgerAccount
	err := DB.Where("owner_type = ? AND owner_id = ? AND kind = ? AND currency = ?",
		ownerType, ownerId, kind, currency).First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return acc.BalanceMicros, nil
}

// SumCreditsSince returns the total credited (incoming) micros to an account
// since the given unix time (inclusive). It reflects gross earnings over a
// window — unlike the account balance, it is unaffected by later debits such as
// withdrawals. Returns 0 if the account does not exist. A zero `since` sums the
// account's whole history (lifetime earnings).
func SumCreditsSince(ownerType string, ownerId int, kind, currency string, since int64) (int64, error) {
	var acc LedgerAccount
	err := DB.Where("owner_type = ? AND owner_id = ? AND kind = ? AND currency = ?",
		ownerType, ownerId, kind, currency).First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var total int64
	q := DB.Model(&LedgerEntry{}).
		Where("account_id = ? AND direction = ?", acc.Id, DirCredit)
	if since > 0 {
		q = q.Where("created_at >= ?", since)
	}
	if err := q.Select("COALESCE(SUM(amount_micros),0)").Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// ReplayBalance recomputes an account's balance from its entries, used by tests
// and reconciliation to assert the cached balance is correct.
func ReplayBalance(accountId int) (int64, error) {
	var entries []LedgerEntry
	if err := DB.Where("account_id = ?", accountId).Find(&entries).Error; err != nil {
		return 0, err
	}
	var bal int64
	for _, e := range entries {
		bal += signedDelta(e.Direction, e.AmountMicros)
	}
	return bal, nil
}

// AssertLedgerBalanced returns an error if global debits != credits across all
// posted entries (a system-wide invariant that must always hold).
func AssertLedgerBalanced() error {
	var debits, credits int64
	if err := DB.Model(&LedgerEntry{}).Where("direction = ?", DirDebit).
		Select("COALESCE(SUM(amount_micros),0)").Scan(&debits).Error; err != nil {
		return err
	}
	if err := DB.Model(&LedgerEntry{}).Where("direction = ?", DirCredit).
		Select("COALESCE(SUM(amount_micros),0)").Scan(&credits).Error; err != nil {
		return err
	}
	if debits != credits {
		return ErrUnbalanced
	}
	return nil
}
