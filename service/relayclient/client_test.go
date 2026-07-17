package relayclient

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/service/dataplane"
	"github.com/QuantumNous/new-api/service/relayhub"
)

// TestRunClientSession_RoundTrip exercises RunClientSession against the real
// relay hub + shared dataplane crypto (no WebSocket). A fake provider mirrors
// the client handshake, decrypts the config and seals a reply, asserting
// byte-level interop with the browser/plugin data-plane protocol.
func TestRunClientSession_RoundTrip(t *testing.T) {
	hub := relayhub.New()
	taskID := "ord_test_1"
	attempt := 1
	clientID := 42
	providerDeviceID := "dev_provider_1"

	priv, err := dataplane.GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	config := []byte(`{"prompt":"a dog","seconds":4}`)
	replyPlaintext := []byte(`{"url":"https://example.com/v.mp4"}`)

	prov := &providerConn{
		hub:              hub,
		taskID:           taskID,
		attempt:          attempt,
		providerDeviceID: providerDeviceID,
		priv:             priv,
		pubB64:           base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes()),
		gotConfig:        make(chan []byte, 1),
		reply:            replyPlaintext,
	}
	hub.Join(taskID, attempt, relayhub.RoleProvider, prov)
	defer hub.Leave(taskID, attempt, relayhub.RoleProvider, prov)

	resultCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := runClientSessionOn(hub, taskID, attempt, clientID, config,
			5*time.Second, 5*time.Second)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- res
	}()

	select {
	case got := <-prov.gotConfig:
		if string(got) != string(config) {
			t.Fatalf("provider got config %q, want %q", got, config)
		}
	case err := <-errCh:
		t.Fatalf("session failed before config delivery: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("provider never received config")
	}

	select {
	case res := <-resultCh:
		if string(res) != string(replyPlaintext) {
			t.Fatalf("client got result %q, want %q", res, replyPlaintext)
		}
	case err := <-errCh:
		t.Fatalf("session returned error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("client never received result")
	}
}

// TestRunClientSession_HandshakeTimeout fails fast when no provider joins.
func TestRunClientSession_HandshakeTimeout(t *testing.T) {
	hub := relayhub.New()
	_, err := runClientSessionOn(hub, "ord_no_provider", 1, 1, []byte(`{}`),
		200*time.Millisecond, 5*time.Second)
	if err != ErrHandshakeTimeout {
		t.Fatalf("want ErrHandshakeTimeout, got %v", err)
	}
}

// providerConn is the fake provider side implementing relayhub.Conn. It derives
// directional keys on the client hello (provider reads c2p, writes p2c).
type providerConn struct {
	hub              *relayhub.Hub
	taskID           string
	attempt          int
	providerDeviceID string
	priv             *ecdh.PrivateKey
	pubB64           string
	gotConfig        chan []byte
	reply            []byte

	sealer *dataplane.Sealer
	opener *dataplane.Opener
}

func (p *providerConn) Close() error { return nil }

func (p *providerConn) SendFrame(data []byte) error {
	if len(data) < 1 {
		return nil
	}
	tag := data[0]
	body := data[1:]
	switch tag {
	case tagHandshake:
		p.onHandshake(body)
	case tagData:
		if p.opener == nil {
			return nil
		}
		pt, err := p.opener.Open(body)
		if err != nil {
			return nil
		}
		p.gotConfig <- pt
		frame := p.sealer.Seal(p.reply)
		_ = p.hub.Forward(p.taskID, p.attempt, relayhub.RoleProvider, tagFrame(tagData, frame))
	}
	return nil
}

func (p *providerConn) onHandshake(body []byte) {
	if p.sealer != nil {
		return
	}
	var msg handshakeMsg
	if err := json.Unmarshal(body, &msg); err != nil || msg.Pub == "" {
		return
	}
	peerPub, err := base64.StdEncoding.DecodeString(msg.Pub)
	if err != nil {
		return
	}
	secret, err := dataplane.SharedSecret(p.priv, peerPub)
	if err != nil {
		return
	}
	ctx := dataplane.SessionContext{
		TaskID:           p.taskID,
		Attempt:          p.attempt,
		ClientDeviceID:   msg.DeviceID,
		ProviderDeviceID: p.providerDeviceID,
	}
	c2p, _ := dataplane.DeriveKey(secret, ctx, dataplane.DirClientToProvider)
	p2c, _ := dataplane.DeriveKey(secret, ctx, dataplane.DirProviderToClient)
	p.opener, _ = dataplane.NewOpener(c2p)
	p.sealer, _ = dataplane.NewSealer(p2c)
	hello, _ := json.Marshal(handshakeMsg{
		Role:     relayhub.RoleProvider,
		Pub:      p.pubB64,
		DeviceID: p.providerDeviceID,
	})
	_ = p.hub.Forward(p.taskID, p.attempt, relayhub.RoleProvider, tagFrame(tagHandshake, hello))
}
