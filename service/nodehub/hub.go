// Package nodehub keeps the in-memory registry of live Provider control-channel
// connections so the Outbox publisher can push task.offer (and other control
// events) to the specific node that owns a task. It is transport-agnostic: a
// connection is any NodeConn, so the hub is unit-tested with fakes while the
// real WSS handler wraps *websocket.Conn.
package nodehub

import (
	"errors"
	"sync"
)

// ErrNodeNotConnected is returned when no live connection exists for a node.
var ErrNodeNotConnected = errors.New("node not connected")

// NodeConn is the minimal write surface the hub needs. The WSS handler adapts
// *websocket.Conn to this; tests use a fake.
type NodeConn interface {
	// SendJSON writes one control event to the node. Must be safe to call from
	// the publisher goroutine.
	SendJSON(v any) error
	// Close terminates the connection (used when a newer connection for the same
	// node supersedes an old one).
	Close() error
}

// Hub tracks at most one live connection per node id. A new connection for a
// node replaces (and closes) the previous one — a node runs one control channel
// at a time, mirroring the single-active-lease rule.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]NodeConn
}

// New returns an empty hub.
func New() *Hub {
	return &Hub{conns: make(map[string]NodeConn)}
}

// Register adds/replaces the connection for a node, closing any prior one.
// Returns the superseded connection (or nil) so the caller can stop its reader.
func (h *Hub) Register(nodeID string, conn NodeConn) NodeConn {
	h.mu.Lock()
	prev := h.conns[nodeID]
	h.conns[nodeID] = conn
	h.mu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}
	return prev
}

// Unregister removes a node's connection, but only if it is still the current
// one (avoids a late-closing old connection evicting a fresh one).
func (h *Hub) Unregister(nodeID string, conn NodeConn) {
	h.mu.Lock()
	if h.conns[nodeID] == conn {
		delete(h.conns, nodeID)
	}
	h.mu.Unlock()
}

// Send delivers a control event to a node's live connection.
func (h *Hub) Send(nodeID string, v any) error {
	h.mu.RLock()
	conn := h.conns[nodeID]
	h.mu.RUnlock()
	if conn == nil {
		return ErrNodeNotConnected
	}
	return conn.SendJSON(v)
}

// IsOnline reports whether a node currently holds a live connection.
func (h *Hub) IsOnline(nodeID string) bool {
	h.mu.RLock()
	_, ok := h.conns[nodeID]
	h.mu.RUnlock()
	return ok
}

// Count returns the number of connected nodes (for metrics/tests).
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

// Default is the process-wide hub used by the WSS handler and publisher.
var Default = New()
