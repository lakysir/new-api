// Package relayhub is the E2EE data-plane relay: it pairs the two sides of a
// task attempt (client and provider) and forwards opaque ciphertext frames
// between them. It holds NO keys and never inspects payloads — only routes by
// (task_id, attempt, direction). This is the server-side of architecture §9's
// "E2EE WebSocket Relay" fallback path.
package relayhub

import (
	"errors"
	"sync"
)

// Role identifies which side of a task a connection represents.
const (
	RoleClient   = "client"
	RoleProvider = "provider"
)

// ErrPeerNotConnected is returned when the other side hasn't joined yet.
var ErrPeerNotConnected = errors.New("relay peer not connected")

const (
	// MaxPlaintextBytes caps the plaintext (config or result) a single relay
	// frame may carry, in either direction. Even though the relay only forwards
	// opaque ciphertext, every frame still crosses the operator's bandwidth and
	// memory, so an unbounded payload (e.g. a base64-encoded image stuffed into
	// the config, or a huge inline result) would let a client or provider
	// saturate the relay. 2 MiB is generous for real config/result JSON: video
	// scripts return a URL, not inline media. Callers should reject oversized
	// plaintext before sealing so the peer sees a clear error.
	MaxPlaintextBytes = 2 * 1024 * 1024
	// MaxFrameBytes is the wire-frame ceiling the relay enforces on each socket.
	// It is MaxPlaintextBytes plus headroom for the AEAD framing overhead
	// (1-byte tag + 8-byte sequence header + 16-byte GCM tag) so a full-size
	// legitimate payload still fits, while anything materially larger is dropped
	// by the WebSocket read limit (which sends a close frame and ends the read).
	MaxFrameBytes = MaxPlaintextBytes + 1024
)

// Conn is the minimal write surface a relayed connection must provide. The WSS
// handler adapts *websocket.Conn; tests use a fake.
type Conn interface {
	// SendFrame forwards one opaque frame to this connection. Safe for
	// concurrent use with the peer's read loop.
	SendFrame(data []byte) error
	Close() error
}

// pair holds the two sides of one task attempt.
type pair struct {
	client   Conn
	provider Conn
}

// key uniquely identifies a task attempt's relay session.
type key struct {
	taskID  string
	attempt int
}

// Hub pairs client/provider connections per task attempt and forwards frames.
type Hub struct {
	mu    sync.RWMutex
	pairs map[key]*pair
}

// New returns an empty relay hub.
func New() *Hub {
	return &Hub{pairs: make(map[key]*pair)}
}

// Join registers a connection for one side of a task attempt, replacing (and
// closing) any prior connection for that same side.
func (h *Hub) Join(taskID string, attempt int, role string, conn Conn) {
	k := key{taskID, attempt}
	h.mu.Lock()
	p := h.pairs[k]
	if p == nil {
		p = &pair{}
		h.pairs[k] = p
	}
	var prev Conn
	if role == RoleClient {
		prev = p.client
		p.client = conn
	} else {
		prev = p.provider
		p.provider = conn
	}
	h.mu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}
}

// Leave removes a side's connection (only if it is still the current one) and
// drops the pair when both sides are gone.
func (h *Hub) Leave(taskID string, attempt int, role string, conn Conn) {
	k := key{taskID, attempt}
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.pairs[k]
	if p == nil {
		return
	}
	if role == RoleClient && p.client == conn {
		p.client = nil
	} else if role == RoleProvider && p.provider == conn {
		p.provider = nil
	}
	if p.client == nil && p.provider == nil {
		delete(h.pairs, k)
	}
}

// Forward sends a frame from `fromRole` to the OTHER side of the task attempt.
// Returns ErrPeerNotConnected if the peer hasn't joined.
func (h *Hub) Forward(taskID string, attempt int, fromRole string, data []byte) error {
	k := key{taskID, attempt}
	h.mu.RLock()
	p := h.pairs[k]
	var peer Conn
	if p != nil {
		if fromRole == RoleClient {
			peer = p.provider
		} else {
			peer = p.client
		}
	}
	h.mu.RUnlock()
	if peer == nil {
		return ErrPeerNotConnected
	}
	return peer.SendFrame(data)
}

// PeerConnected reports whether the other side has joined.
func (h *Hub) PeerConnected(taskID string, attempt int, fromRole string) bool {
	k := key{taskID, attempt}
	h.mu.RLock()
	defer h.mu.RUnlock()
	p := h.pairs[k]
	if p == nil {
		return false
	}
	if fromRole == RoleClient {
		return p.provider != nil
	}
	return p.client != nil
}

// Default is the process-wide relay hub.
var Default = New()
