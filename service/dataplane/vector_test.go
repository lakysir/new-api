package dataplane

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type dpVector struct {
	ClientPrivB64   string `json:"client_priv_b64"`
	ClientPubB64    string `json:"client_pub_b64"`
	ProviderPrivB64 string `json:"provider_priv_b64"`
	ProviderPubB64  string `json:"provider_pub_b64"`
	SharedSecretB64 string `json:"shared_secret_b64"`
	C2PKeyB64       string `json:"c2p_key_b64"`
	Context         struct {
		TaskID           string `json:"task_id"`
		Attempt          int    `json:"attempt"`
		ClientDeviceID   string `json:"client_device_id"`
		ProviderDeviceID string `json:"provider_device_id"`
	} `json:"context"`
	Plaintext string `json:"plaintext"`
	FrameB64  string `json:"frame_b64"`
}

func loadDPVector(t *testing.T) dpVector {
	t.Helper()
	p := filepath.Join("..", "..", "..", "client-sdk", "protocol", "dataplane_vector.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Skipf("vector fixture unavailable: %v", err)
	}
	var v dpVector
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func b64d(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestVectorReproducible re-derives shared secret, key and frame from the fixed
// private keys and asserts they match the committed vector — the same bytes the
// TypeScript SDK is tested against.
func TestVectorReproducible(t *testing.T) {
	v := loadDPVector(t)
	clientPriv, err := ecdh.X25519().NewPrivateKey(b64d(t, v.ClientPrivB64))
	if err != nil {
		t.Fatal(err)
	}
	shared, err := SharedSecret(clientPriv, b64d(t, v.ProviderPubB64))
	if err != nil {
		t.Fatal(err)
	}
	if base64.StdEncoding.EncodeToString(shared) != v.SharedSecretB64 {
		t.Fatal("shared secret drift from vector")
	}
	ctx := SessionContext{
		TaskID: v.Context.TaskID, Attempt: v.Context.Attempt,
		ClientDeviceID: v.Context.ClientDeviceID, ProviderDeviceID: v.Context.ProviderDeviceID,
	}
	key, _ := DeriveKey(shared, ctx, DirClientToProvider)
	if base64.StdEncoding.EncodeToString(key) != v.C2PKeyB64 {
		t.Fatal("c2p key drift from vector")
	}
	sealer, _ := NewSealer(key)
	frame := sealer.Seal([]byte(v.Plaintext))
	if base64.StdEncoding.EncodeToString(frame) != v.FrameB64 {
		t.Fatalf("frame drift from vector:\n got %s\nwant %s", base64.StdEncoding.EncodeToString(frame), v.FrameB64)
	}
}

// TestVectorProviderOpens confirms the provider side (its private key + client
// public) derives the same key and opens the committed frame.
func TestVectorProviderOpens(t *testing.T) {
	v := loadDPVector(t)
	provPriv, err := ecdh.X25519().NewPrivateKey(b64d(t, v.ProviderPrivB64))
	if err != nil {
		t.Fatal(err)
	}
	shared, err := SharedSecret(provPriv, b64d(t, v.ClientPubB64))
	if err != nil {
		t.Fatal(err)
	}
	ctx := SessionContext{
		TaskID: v.Context.TaskID, Attempt: v.Context.Attempt,
		ClientDeviceID: v.Context.ClientDeviceID, ProviderDeviceID: v.Context.ProviderDeviceID,
	}
	key, _ := DeriveKey(shared, ctx, DirClientToProvider)
	opener, _ := NewOpener(key)
	pt, err := opener.Open(b64d(t, v.FrameB64))
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != v.Plaintext {
		t.Fatalf("opened plaintext mismatch: %q", string(pt))
	}
}
