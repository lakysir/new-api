package dispatch

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// recordingSender captures deliveries for assertions.
type recordingSender struct {
	sent map[string]int // nodeId -> count
	fail map[string]bool
}

func newRecordingSender() *recordingSender {
	return &recordingSender{sent: map[string]int{}, fail: map[string]bool{}}
}

func (s *recordingSender) Send(nodeID string, _ any) error {
	if s.fail[nodeID] {
		return assertErr
	}
	s.sent[nodeID]++
	return nil
}

var assertErr = &sendErr{}

type sendErr struct{}

func (*sendErr) Error() string { return "send failed" }

// enqueueOfferFor creates a task attempt on a node and an outbox task.offer for
// it, mirroring what Dispatch does, so PublishBatch has something to route.
func enqueueOfferFor(t *testing.T, taskId, orderId, nodeId string, attempt int) {
	t.Helper()
	if err := model.DB.Create(&model.TaskAttempt{
		TaskId: taskId, OrderId: orderId, Attempt: attempt, NodeId: nodeId, State: model.AttemptReserved,
	}).Error; err != nil {
		t.Fatal(err)
	}
	payload := `{"task_id":"` + taskId + `","attempt":` + itoa(attempt) + `,"order_id":"` + orderId + `"}`
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		_, e := model.EnqueueOutboxTx(tx, "task.offer", taskId, payload)
		return e
	}); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestPublishDeliversToOwningNode(t *testing.T) {
	enqueueOfferFor(t, "tsk_pub1", "ord_pub1", "node_pub1", 1)
	sender := newRecordingSender()
	delivered, _, err := PublishBatch(sender, 100)
	if err != nil {
		t.Fatal(err)
	}
	if delivered < 1 || sender.sent["node_pub1"] != 1 {
		t.Fatalf("offer must be delivered to owning node, got delivered=%d sent=%v", delivered, sender.sent)
	}
	// Re-publishing must not resend (event marked published).
	sender2 := newRecordingSender()
	d2, _, _ := PublishBatch(sender2, 100)
	if sender2.sent["node_pub1"] != 0 {
		t.Fatalf("published event must not resend, got %v (d2=%d)", sender2.sent, d2)
	}
}

func TestPublishEventDeliversOnlyRequestedOffer(t *testing.T) {
	enqueueOfferFor(t, "tsk_direct", "ord_direct", "node_direct", 1)
	events, err := model.FetchUnpublished(100)
	if err != nil {
		t.Fatal(err)
	}
	var eventID string
	for _, event := range events {
		if event.AggregateId == "tsk_direct" {
			eventID = event.EventId
			break
		}
	}
	if eventID == "" {
		t.Fatal("direct offer event not found")
	}
	sender := newRecordingSender()
	if err := PublishEvent(sender, eventID); err != nil {
		t.Fatal(err)
	}
	if sender.sent["node_direct"] != 1 {
		t.Fatalf("direct offer must be delivered once, got %v", sender.sent)
	}
	if err := PublishEvent(sender, eventID); err != nil {
		t.Fatal(err)
	}
	if sender.sent["node_direct"] != 1 {
		t.Fatalf("published direct offer must not be resent, got %v", sender.sent)
	}
}

func TestPublishSkipsOfflineNodeAndRetriesLater(t *testing.T) {
	enqueueOfferFor(t, "tsk_pub2", "ord_pub2", "node_pub2", 1)
	// Node offline: sender fails, event stays unpublished.
	failing := newRecordingSender()
	failing.fail["node_pub2"] = true
	_, skipped, err := PublishBatch(failing, 100)
	if err != nil {
		t.Fatal(err)
	}
	if skipped < 1 {
		t.Fatalf("offline node delivery must be skipped, got skipped=%d", skipped)
	}
	// Later, node online: the same event is retried and delivered.
	ok := newRecordingSender()
	delivered, _, _ := PublishBatch(ok, 100)
	if delivered < 1 || ok.sent["node_pub2"] != 1 {
		t.Fatalf("retry must deliver once node is online, got %v", ok.sent)
	}
}

// ensure the time import is used (StartPublisher signature reference).
var _ = time.Second
