// Package relayclient is the in-process client side of the E2EE data plane. It
// lets the marketplace bridge adaptor drive a task exactly like the browser
// purchase page does — join the relay for (task_id, attempt) as "client",
// handshake with the provider (ephemeral X25519 keys), send the encrypted
// config and await the encrypted result — without ever going over a WebSocket
// or exposing keys to the relay. It mirrors web client-relay-session.ts and the
// plugin's RelaySession, and reuses the shared dataplane crypto so ciphertext
// interops byte-for-byte with the provider.
package relayclient

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/dataplane"
	"github.com/QuantumNous/new-api/service/relayhub"
)

// Frame tags mirror the browser/plugin data plane: 0x01 = handshake JSON,
// 0x02 = AEAD data frame. The 1-byte tag prefixes every relay frame.
const (
	tagHandshake byte = 0x01
	tagData      byte = 0x02
)

// Default timeouts mirror the browser client (client-relay-session.ts).
const (
	defaultHandshakeTimeout = 30 * time.Second
	defaultResultTimeout    = 120 * time.Second
)

// ErrHandshakeTimeout / ErrResultTimeout surface the two waits the caller may
// hit; the adaptor maps them to a task failure reason.
var (
	ErrHandshakeTimeout = errors.New("relay handshake timeout")
	ErrResultTimeout    = errors.New("relay result timeout")
)

// handshakeMsg is the in-band JSON exchanged before AEAD frames start.
type handshakeMsg struct {
	Role     string `json:"role"`
	Pub      string `json:"pub"`
	DeviceID string `json:"device_id"`
}

// session is one client-side relay session. It implements relayhub.Conn so the
// hub can deliver provider frames to us in-process (SendFrame), while we send
// to the provider via hub.Forward.
type session struct {
	hub     *relayhub.Hub
	taskID  string
	attempt int

	clientDeviceID string
	priv           *ecdh.PrivateKey
	pubB64         string

	mu     sync.Mutex
	sealer *dataplane.Sealer
	opener *dataplane.Opener

	established chan struct{}
	establishOnce sync.Once
	result        chan []byte
	resultOnce    sync.Once
	pending       [][]byte // data frames that arrived before keys were derived
	closed        bool
}

func tagFrame(tag byte, body []byte) []byte {
	out := make([]byte, 1+len(body))
	out[0] = tag
	copy(out[1:], body)
	return out
}

// SendFrame is called by the relay hub with a frame the provider sent us.
func (s *session) SendFrame(data []byte) error {
	if len(data) < 1 {
		return nil
	}
	tag := data[0]
	body := data[1:]
	switch tag {
	case tagHandshake:
		s.onHandshake(body)
	case tagData:
		s.mu.Lock()
		if s.opener == nil {
			// Keys not derived yet; buffer until the handshake completes.
			s.pending = append(s.pending, append([]byte(nil), body...))
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
		s.onData(body)
	}
	return nil
}

func (s *session) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

// sendHandshake forwards our client hello to the provider through the hub.
func (s *session) sendHandshake() {
	hello, _ := common.Marshal(handshakeMsg{
		Role:     relayhub.RoleClient,
		Pub:      s.pubB64,
		DeviceID: s.clientDeviceID,
	})
	_ = s.hub.Forward(s.taskID, s.attempt, relayhub.RoleClient, tagFrame(tagHandshake, hello))
}

// onHandshake derives the directional session keys from the provider's public
// key and drains any buffered data frames. Idempotent: a duplicate/late
// handshake after keys are set is ignored so the Opener sequence is preserved.
func (s *session) onHandshake(body []byte) {
	var msg handshakeMsg
	if err := common.Unmarshal(body, &msg); err != nil {
		return
	}
	if msg.Pub == "" || msg.DeviceID == "" {
		return
	}
	s.mu.Lock()
	if s.sealer != nil {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	peerPub, err := base64.StdEncoding.DecodeString(msg.Pub)
	if err != nil {
		return
	}
	secret, err := dataplane.SharedSecret(s.priv, peerPub)
	if err != nil {
		return
	}
	ctx := dataplane.SessionContext{
		TaskID:           s.taskID,
		Attempt:          s.attempt,
		ClientDeviceID:   s.clientDeviceID,
		ProviderDeviceID: msg.DeviceID,
	}
	// Client writes c2p, reads p2c.
	c2p, err := dataplane.DeriveKey(secret, ctx, dataplane.DirClientToProvider)
	if err != nil {
		return
	}
	p2c, err := dataplane.DeriveKey(secret, ctx, dataplane.DirProviderToClient)
	if err != nil {
		return
	}
	sealer, err := dataplane.NewSealer(c2p)
	if err != nil {
		return
	}
	opener, err := dataplane.NewOpener(p2c)
	if err != nil {
		return
	}

	s.mu.Lock()
	if s.sealer != nil { // lost a race; keep the first
		s.mu.Unlock()
		return
	}
	s.sealer = sealer
	s.opener = opener
	pending := s.pending
	s.pending = nil
	s.mu.Unlock()

	s.establishOnce.Do(func() { close(s.established) })
	for _, frame := range pending {
		s.onData(frame)
	}
}

// onData decrypts a provider frame and delivers the first successful plaintext
// as the result.
func (s *session) onData(body []byte) {
	s.mu.Lock()
	opener := s.opener
	s.mu.Unlock()
	if opener == nil {
		return
	}
	pt, err := opener.Open(body)
	if err != nil {
		return // ignore malformed/duplicate
	}
	s.resultOnce.Do(func() { s.result <- pt })
}

// RunClientSession joins the relay as the client for (orderId, attempt=1),
// completes the handshake with the provider, sends the config JSON encrypted,
// and returns the decrypted result bytes. It never blocks longer than the
// handshake + result timeouts. The caller is responsible for order lifecycle
// (reserve/dispatch/settle) around this call.
func RunClientSession(orderID string, clientID int, config []byte) ([]byte, error) {
	return runClientSessionOn(relayhub.Default, orderID, 1, clientID, config,
		defaultHandshakeTimeout, defaultResultTimeout)
}

// runClientSessionOn is the injectable core (hub + timeouts) used by tests.
func runClientSessionOn(hub *relayhub.Hub, orderID string, attempt, clientID int,
	config []byte, handshakeTimeout, resultTimeout time.Duration) ([]byte, error) {
	priv, err := dataplane.GenerateKeyPair(rand.Reader)
	if err != nil {
		return nil, err
	}
	s := &session{
		hub:            hub,
		taskID:         orderID,
		attempt:        attempt,
		clientDeviceID: fmt.Sprintf("client-%d", clientID),
		priv:           priv,
		pubB64:         base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes()),
		established:    make(chan struct{}),
		result:         make(chan []byte, 1),
	}
	hub.Join(orderID, attempt, relayhub.RoleClient, s)
	defer hub.Leave(orderID, attempt, relayhub.RoleClient, s)

	// Send our hello immediately, then re-send until the provider joins and we
	// derive keys. The provider mirrors this; whoever joined second triggers the
	// pairing. Stop as soon as keys are established or we time out.
	s.sendHandshake()
	stopResend := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopResend:
				return
			case <-ticker.C:
				s.mu.Lock()
				established := s.sealer != nil
				s.mu.Unlock()
				if established {
					return
				}
				s.sendHandshake()
			}
		}
	}()

	select {
	case <-s.established:
		close(stopResend)
	case <-time.After(handshakeTimeout):
		close(stopResend)
		return nil, ErrHandshakeTimeout
	}

	// Encrypt and send the config to the provider.
	s.mu.Lock()
	frame := s.sealer.Seal(config)
	s.mu.Unlock()
	if err := hub.Forward(orderID, attempt, relayhub.RoleClient, tagFrame(tagData, frame)); err != nil {
		return nil, err
	}

	select {
	case pt := <-s.result:
		return pt, nil
	case <-time.After(resultTimeout):
		return nil, ErrResultTimeout
	}
}
