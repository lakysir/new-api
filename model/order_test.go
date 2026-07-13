package model

import (
	"sync"
	"testing"
)

func makeTestOrder(t *testing.T, key string) *Order {
	t.Helper()
	o := &Order{
		Id:              NewOrderId(),
		ClientId:        700,
		ScriptId:        1,
		Version:         1,
		State:           OrderCreated,
		IdempotencyKey:  key,
		MaxAmountMicros: 112000,
	}
	snap := &OrderPriceSnapshot{
		Currency: "USD", ProviderAmountMicros: 100000, AuthorAmountMicros: 3000,
		PlatformFeeMicros: 8000, RiskReserveMicros: 1000, MaxCustomerAmountMicros: 112000,
	}
	got, _, err := CreateOrderWithSnapshot(o, snap)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCanTransitionTable(t *testing.T) {
	legal := [][2]string{
		{OrderCreated, OrderFundsReserved},
		{OrderFundsReserved, OrderMatching},
		{OrderMatching, OrderOffered},
		{OrderOffered, OrderReserved},
		{OrderReserved, OrderDataReady},
		{OrderRunning, OrderResultReady},
		{OrderVerifying, OrderSettled},
	}
	for _, p := range legal {
		if !CanTransition(p[0], p[1]) {
			t.Fatalf("expected %s -> %s legal", p[0], p[1])
		}
	}
	illegal := [][2]string{
		{OrderCreated, OrderRunning},
		{OrderSettled, OrderMatching},
		{OrderRefunded, OrderRunning},
		{OrderMatching, OrderSettled},
		{OrderRunning, OrderSettled},
	}
	for _, p := range illegal {
		if CanTransition(p[0], p[1]) {
			t.Fatalf("expected %s -> %s illegal", p[0], p[1])
		}
	}
}

func TestApplyTransitionRejectsIllegal(t *testing.T) {
	o := makeTestOrder(t, "key-illegal-1")
	if _, err := ApplyTransition(o.Id, OrderRunning, nil); err != ErrIllegalTransition {
		t.Fatalf("expected ErrIllegalTransition, got %v", err)
	}
}

func TestApplyTransitionLegalBumpsLockVersion(t *testing.T) {
	o := makeTestOrder(t, "key-legal-1")
	updated, err := ApplyTransition(o.Id, OrderFundsReserved, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != OrderFundsReserved {
		t.Fatal("state not updated")
	}
	if updated.LockVersion != o.LockVersion+1 {
		t.Fatalf("lock version should bump: %d -> %d", o.LockVersion, updated.LockVersion)
	}
}

func TestConcurrentTransitionsSerialize(t *testing.T) {
	o := makeTestOrder(t, "key-concurrent-1")
	// Many goroutines race to do CREATED -> FUNDS_RESERVED. Exactly one should
	// win; the rest hit the optimistic lock or find the state already moved.
	const n = 30
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ApplyTransition(o.Id, OrderFundsReserved, nil); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("exactly one transition should succeed, got %d", wins)
	}
}

func TestCreateOrderIdempotent(t *testing.T) {
	key := "idem-key-xyz"
	o1 := makeTestOrder(t, key)
	// Second create with the same key must return the same order, not a new one.
	o2 := &Order{
		Id: NewOrderId(), ClientId: 700, ScriptId: 1, Version: 1,
		State: OrderCreated, IdempotencyKey: key, MaxAmountMicros: 999999,
	}
	snap := &OrderPriceSnapshot{Currency: "USD", MaxCustomerAmountMicros: 999999}
	got, created, err := CreateOrderWithSnapshot(o2, snap)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("duplicate idempotency key must not create a new order")
	}
	if got.Id != o1.Id {
		t.Fatalf("expected same order id %s, got %s", o1.Id, got.Id)
	}
	if got.MaxAmountMicros != o1.MaxAmountMicros {
		t.Fatal("existing order must be returned unchanged")
	}
}

func TestConcurrentCreateSameKeyOneOrder(t *testing.T) {
	key := "idem-race-key"
	const n = 20
	var wg sync.WaitGroup
	ids := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o := &Order{
				Id: NewOrderId(), ClientId: 701, ScriptId: 1, Version: 1,
				State: OrderCreated, IdempotencyKey: key, MaxAmountMicros: 100,
			}
			snap := &OrderPriceSnapshot{Currency: "USD", MaxCustomerAmountMicros: 100}
			got, _, err := CreateOrderWithSnapshot(o, snap)
			if err == nil && got != nil {
				ids <- got.Id
			}
		}()
	}
	wg.Wait()
	close(ids)
	unique := map[string]bool{}
	for id := range ids {
		unique[id] = true
	}
	if len(unique) != 1 {
		t.Fatalf("concurrent creates with same key must yield one order, got %d distinct ids", len(unique))
	}
}

func TestOrderPriceSnapshotPersisted(t *testing.T) {
	o := makeTestOrder(t, "key-snap-1")
	snap, err := GetOrderPriceSnapshot(o.Id)
	if err != nil {
		t.Fatal(err)
	}
	// Snapshot components must sum to the max customer amount (§14.4 invariant).
	sum := snap.ProviderAmountMicros + snap.AuthorAmountMicros + snap.PlatformFeeMicros +
		snap.RelayFeeReservedMicros + snap.StorageFeeReservedMicros + snap.RiskReserveMicros
	if sum != snap.MaxCustomerAmountMicros {
		t.Fatalf("snapshot components %d != max %d", sum, snap.MaxCustomerAmountMicros)
	}
}
