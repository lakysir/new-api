package relayhub

import (
	"sync"
	"testing"
)

type fakeConn struct {
	mu     sync.Mutex
	frames [][]byte
	closed bool
}

func (f *fakeConn) SendFrame(d []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames = append(f.frames, d)
	return nil
}
func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func TestForwardBetweenSides(t *testing.T) {
	h := New()
	client := &fakeConn{}
	provider := &fakeConn{}
	h.Join("tsk_1", 1, RoleClient, client)
	h.Join("tsk_1", 1, RoleProvider, provider)

	// Client -> provider.
	if err := h.Forward("tsk_1", 1, RoleClient, []byte("cfg-cipher")); err != nil {
		t.Fatal(err)
	}
	if len(provider.frames) != 1 || string(provider.frames[0]) != "cfg-cipher" {
		t.Fatalf("provider should receive the client frame, got %v", provider.frames)
	}
	// Provider -> client.
	if err := h.Forward("tsk_1", 1, RoleProvider, []byte("result-cipher")); err != nil {
		t.Fatal(err)
	}
	if len(client.frames) != 1 || string(client.frames[0]) != "result-cipher" {
		t.Fatalf("client should receive the provider frame, got %v", client.frames)
	}
}

func TestForwardWithoutPeerFails(t *testing.T) {
	h := New()
	h.Join("tsk_2", 1, RoleClient, &fakeConn{})
	if err := h.Forward("tsk_2", 1, RoleClient, []byte("x")); err != ErrPeerNotConnected {
		t.Fatalf("expected ErrPeerNotConnected, got %v", err)
	}
}

func TestSessionsAreIsolatedByTaskAttempt(t *testing.T) {
	h := New()
	p1 := &fakeConn{}
	p2 := &fakeConn{}
	h.Join("tsk_3", 1, RoleClient, &fakeConn{})
	h.Join("tsk_3", 1, RoleProvider, p1)
	h.Join("tsk_3", 2, RoleClient, &fakeConn{})
	h.Join("tsk_3", 2, RoleProvider, p2)
	// A frame in attempt 1 must not leak into attempt 2.
	_ = h.Forward("tsk_3", 1, RoleClient, []byte("a1"))
	if len(p2.frames) != 0 {
		t.Fatal("attempt 2 must not receive attempt 1 frames")
	}
	if len(p1.frames) != 1 {
		t.Fatal("attempt 1 provider should receive its frame")
	}
}

func TestLeaveDropsPairAndClosesReplaced(t *testing.T) {
	h := New()
	old := &fakeConn{}
	h.Join("tsk_4", 1, RoleClient, old)
	fresh := &fakeConn{}
	h.Join("tsk_4", 1, RoleClient, fresh) // supersede
	if !old.closed {
		t.Fatal("replaced connection must be closed")
	}
	h.Leave("tsk_4", 1, RoleClient, fresh)
	// Peer of a now-empty session: forward should report not connected.
	if h.PeerConnected("tsk_4", 1, RoleProvider) {
		t.Fatal("no client should remain after leave")
	}
}
