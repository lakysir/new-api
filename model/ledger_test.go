package model

import (
	"sync"
	"testing"
)

const testCur = "USD_TEST"

func entry(ot string, oid int, kind, dir string, amt int64) EntryInput {
	return EntryInput{OwnerType: ot, OwnerId: oid, Kind: kind, Currency: testCur, Direction: dir, Amount: amt}
}

func TestPostBalancedTransactionUpdatesBalances(t *testing.T) {
	_, err := PostTransaction("t-deposit-1", TxDeposit, "ref1", []EntryInput{
		entry(OwnerExternal, 0, KindSource, DirDebit, 100000),
		entry(OwnerClient, 900, KindAvailable, DirCredit, 100000),
	})
	if err != nil {
		t.Fatal(err)
	}
	bal, _ := GetBalance(OwnerClient, 900, KindAvailable, testCur)
	if bal != 100000 {
		t.Fatalf("expected client available 100000, got %d", bal)
	}
	if err := AssertLedgerBalanced(); err != nil {
		t.Fatalf("global ledger must be balanced: %v", err)
	}
}

func TestUnbalancedTransactionRejected(t *testing.T) {
	_, err := PostTransaction("t-unbalanced", TxDeposit, "ref", []EntryInput{
		entry(OwnerExternal, 0, KindSource, DirDebit, 100),
		entry(OwnerClient, 901, KindAvailable, DirCredit, 99),
	})
	if err != ErrUnbalanced {
		t.Fatalf("expected ErrUnbalanced, got %v", err)
	}
}

func TestNonPositiveAmountRejected(t *testing.T) {
	_, err := PostTransaction("t-zero", TxDeposit, "ref", []EntryInput{
		entry(OwnerExternal, 0, KindSource, DirDebit, 0),
		entry(OwnerClient, 902, KindAvailable, DirCredit, 0),
	})
	if err != ErrNonPositiveAmount {
		t.Fatalf("expected ErrNonPositiveAmount, got %v", err)
	}
}

func TestInsufficientBalanceRejected(t *testing.T) {
	// Client 903 has no funds; debiting available must fail.
	_, err := PostTransaction("t-insufficient", TxReserve, "ref", []EntryInput{
		entry(OwnerClient, 903, KindAvailable, DirDebit, 5000),
		entry(OwnerClient, 903, KindReserved, DirCredit, 5000),
	})
	if err != ErrInsufficientBalance {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
}

func TestPostTransactionIdempotent(t *testing.T) {
	key := "t-idem-deposit"
	e := []EntryInput{
		entry(OwnerExternal, 0, KindSource, DirDebit, 50000),
		entry(OwnerClient, 904, KindAvailable, DirCredit, 50000),
	}
	tx1, err := PostTransaction(key, TxDeposit, "ref", e)
	if err != nil {
		t.Fatal(err)
	}
	tx2, err := PostTransaction(key, TxDeposit, "ref", e)
	if err != nil {
		t.Fatal(err)
	}
	if tx1.Id != tx2.Id {
		t.Fatal("same idempotency key must return the same transaction")
	}
	// Balance must reflect a single deposit, not two.
	bal, _ := GetBalance(OwnerClient, 904, KindAvailable, testCur)
	if bal != 50000 {
		t.Fatalf("idempotent deposit must credit once, got %d", bal)
	}
}

func TestConcurrentSameKeyPostsOnce(t *testing.T) {
	key := "t-idem-race"
	e := []EntryInput{
		entry(OwnerExternal, 0, KindSource, DirDebit, 1000),
		entry(OwnerClient, 905, KindAvailable, DirCredit, 1000),
	}
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = PostTransaction(key, TxDeposit, "ref", e) }()
	}
	wg.Wait()
	bal, _ := GetBalance(OwnerClient, 905, KindAvailable, testCur)
	if bal != 1000 {
		t.Fatalf("concurrent same-key posts must credit exactly once, got %d", bal)
	}
}

func TestCachedBalanceMatchesReplay(t *testing.T) {
	// A few operations on client 906, then assert cached == replayed.
	_, _ = PostTransaction("t-rep-1", TxDeposit, "r", []EntryInput{
		entry(OwnerExternal, 0, KindSource, DirDebit, 30000),
		entry(OwnerClient, 906, KindAvailable, DirCredit, 30000),
	})
	_, _ = PostTransaction("t-rep-2", TxReserve, "r", []EntryInput{
		entry(OwnerClient, 906, KindAvailable, DirDebit, 12000),
		entry(OwnerClient, 906, KindReserved, DirCredit, 12000),
	})
	var acc LedgerAccount
	if err := DB.Where("owner_type = ? AND owner_id = ? AND kind = ? AND currency = ?",
		OwnerClient, 906, KindAvailable, testCur).First(&acc).Error; err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplayBalance(acc.Id)
	if err != nil {
		t.Fatal(err)
	}
	if replayed != acc.BalanceMicros {
		t.Fatalf("cached balance %d != replayed %d", acc.BalanceMicros, replayed)
	}
	if acc.BalanceMicros != 18000 {
		t.Fatalf("expected 18000 available after reserve, got %d", acc.BalanceMicros)
	}
}
