package model

import (
	"sync"
	"testing"

	"gorm.io/gorm"
)

func TestOutboxEnqueueAndFetch(t *testing.T) {
	err := DB.Transaction(func(tx *gorm.DB) error {
		_, e := EnqueueOutboxTx(tx, "task.offer", "tsk_ob1", `{"x":1}`)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := FetchUnpublished(100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.AggregateId == "tsk_ob1" {
			found = true
			if err := MarkPublished(e.EventId); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !found {
		t.Fatal("enqueued event should appear in unpublished list")
	}
	// After marking published, it must not reappear.
	events2, _ := FetchUnpublished(100)
	for _, e := range events2 {
		if e.AggregateId == "tsk_ob1" {
			t.Fatal("published event must not be refetched")
		}
	}
}

func TestConsumeOnceRunsSideEffectExactlyOnce(t *testing.T) {
	eventId := NewEventId()
	var runs int32
	var mu sync.Mutex
	handler := func(tx *gorm.DB) error {
		mu.Lock()
		runs++
		mu.Unlock()
		return nil
	}
	// Deliver the same event many times concurrently.
	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ConsumeOnce(eventId, "test-consumer", handler)
		}()
	}
	wg.Wait()
	if runs != 1 {
		t.Fatalf("side effect must run exactly once, ran %d times", runs)
	}
}

func TestConsumeOnceDifferentConsumersEachRun(t *testing.T) {
	eventId := NewEventId()
	ran := map[string]bool{}
	for _, consumer := range []string{"c1", "c2"} {
		if err := ConsumeOnce(eventId, consumer, func(tx *gorm.DB) error {
			ran[consumer] = true
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if !ran["c1"] || !ran["c2"] {
		t.Fatal("each distinct consumer must process the event once")
	}
}
