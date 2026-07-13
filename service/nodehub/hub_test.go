package nodehub

import (
	"errors"
	"sync"
	"testing"
)

// fakeConn records sent messages and close calls.
type fakeConn struct {
	mu     sync.Mutex
	sent   []any
	closed bool
}

func (f *fakeConn) SendJSON(v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("closed")
	}
	f.sent = append(f.sent, v)
	return nil
}

func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func TestSendToConnectedNode(t *testing.T) {
	h := New()
	c := &fakeConn{}
	h.Register("node_1", c)
	if err := h.Send("node_1", map[string]string{"type": "task.offer"}); err != nil {
		t.Fatalf("send should succeed: %v", err)
	}
	if len(c.sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(c.sent))
	}
}

func TestSendToUnknownNode(t *testing.T) {
	h := New()
	if err := h.Send("ghost", "x"); err != ErrNodeNotConnected {
		t.Fatalf("expected ErrNodeNotConnected, got %v", err)
	}
}

func TestRegisterSupersedesAndClosesPrevious(t *testing.T) {
	h := New()
	old := &fakeConn{}
	fresh := &fakeConn{}
	h.Register("node_2", old)
	prev := h.Register("node_2", fresh)
	if prev != old {
		t.Fatal("register should return the superseded connection")
	}
	if !old.closed {
		t.Fatal("old connection must be closed on supersede")
	}
	if !h.IsOnline("node_2") || h.Count() != 1 {
		t.Fatal("node should still be online with exactly one connection")
	}
	// Sends go to the fresh connection.
	_ = h.Send("node_2", "hi")
	if len(fresh.sent) != 1 || len(old.sent) != 0 {
		t.Fatal("send must target the fresh connection")
	}
}

func TestUnregisterOnlyRemovesCurrent(t *testing.T) {
	h := New()
	old := &fakeConn{}
	fresh := &fakeConn{}
	h.Register("node_3", old)
	h.Register("node_3", fresh)
	// A late Unregister from the old connection must NOT evict the fresh one.
	h.Unregister("node_3", old)
	if !h.IsOnline("node_3") {
		t.Fatal("stale unregister must not evict the current connection")
	}
	h.Unregister("node_3", fresh)
	if h.IsOnline("node_3") {
		t.Fatal("current unregister should remove the node")
	}
}
