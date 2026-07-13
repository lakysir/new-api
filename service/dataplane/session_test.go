package dataplane

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func testContext() SessionContext {
	return SessionContext{TaskID: "tsk_1", Attempt: 1, ClientDeviceID: "dev_c", ProviderDeviceID: "dev_p"}
}

// establish performs the ECDH + per-direction key derivation for both parties
// and returns their sealers/openers wired to the matching directions.
func establish(t *testing.T) (clientSeal, provSeal *Sealer, clientOpen, provOpen *Opener) {
	t.Helper()
	clientPriv, err := GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	provPriv, err := GenerateKeyPair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ctx := testContext()

	clientShared, err := SharedSecret(clientPriv, provPriv.PublicKey().Bytes())
	if err != nil {
		t.Fatal(err)
	}
	provShared, err := SharedSecret(provPriv, clientPriv.PublicKey().Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(clientShared, provShared) {
		t.Fatal("ECDH shared secrets must match")
	}

	c2pKeyC, _ := DeriveKey(clientShared, ctx, DirClientToProvider)
	p2cKeyC, _ := DeriveKey(clientShared, ctx, DirProviderToClient)
	c2pKeyP, _ := DeriveKey(provShared, ctx, DirClientToProvider)
	p2cKeyP, _ := DeriveKey(provShared, ctx, DirProviderToClient)

	clientSeal, _ = NewSealer(c2pKeyC) // client sends on c2p
	provOpen, _ = NewOpener(c2pKeyP)   // provider reads c2p
	provSeal, _ = NewSealer(p2cKeyP)   // provider sends on p2c
	clientOpen, _ = NewOpener(p2cKeyC) // client reads p2c
	return
}

func TestRoundTripBothDirections(t *testing.T) {
	clientSeal, provSeal, clientOpen, provOpen := establish(t)

	msg1 := []byte(`{"config":"secret"}`)
	frame := clientSeal.Seal(msg1)
	got, err := provOpen.Open(frame)
	if err != nil || !bytes.Equal(got, msg1) {
		t.Fatalf("c2p round-trip failed: %v", err)
	}

	msg2 := []byte(`{"result":"data"}`)
	frame2 := provSeal.Seal(msg2)
	got2, err := clientOpen.Open(frame2)
	if err != nil || !bytes.Equal(got2, msg2) {
		t.Fatalf("p2c round-trip failed: %v", err)
	}
}

func TestDirectionKeysDiffer(t *testing.T) {
	ctx := testContext()
	secret := make([]byte, 32)
	c2p, _ := DeriveKey(secret, ctx, DirClientToProvider)
	p2c, _ := DeriveKey(secret, ctx, DirProviderToClient)
	if bytes.Equal(c2p, p2c) {
		t.Fatal("direction keys must differ")
	}
}

func TestContextBindingChangesKey(t *testing.T) {
	secret := make([]byte, 32)
	a, _ := DeriveKey(secret, SessionContext{TaskID: "t1", Attempt: 1}, DirClientToProvider)
	b, _ := DeriveKey(secret, SessionContext{TaskID: "t2", Attempt: 1}, DirClientToProvider)
	if bytes.Equal(a, b) {
		t.Fatal("different task ids must derive different keys")
	}
	c, _ := DeriveKey(secret, SessionContext{TaskID: "t1", Attempt: 2}, DirClientToProvider)
	if bytes.Equal(a, c) {
		t.Fatal("different attempts must derive different keys")
	}
}

func TestReplayRejected(t *testing.T) {
	clientSeal, _, _, provOpen := establish(t)
	f1 := clientSeal.Seal([]byte("m1"))
	if _, err := provOpen.Open(f1); err != nil {
		t.Fatal(err)
	}
	// Replaying the same frame must fail (sequence already consumed).
	if _, err := provOpen.Open(f1); err != ErrDecrypt {
		t.Fatalf("replay must be rejected, got %v", err)
	}
}

func TestOutOfOrderRejected(t *testing.T) {
	clientSeal, _, _, provOpen := establish(t)
	_ = clientSeal.Seal([]byte("m0"))   // seq 0, not delivered
	f1 := clientSeal.Seal([]byte("m1")) // seq 1
	// Delivering seq 1 before seq 0 must fail.
	if _, err := provOpen.Open(f1); err != ErrDecrypt {
		t.Fatalf("out-of-order must be rejected, got %v", err)
	}
}

func TestTamperRejected(t *testing.T) {
	clientSeal, _, _, provOpen := establish(t)
	frame := clientSeal.Seal([]byte("hello"))
	frame[len(frame)-1] ^= 0xFF // flip a ciphertext/tag bit
	if _, err := provOpen.Open(frame); err != ErrDecrypt {
		t.Fatalf("tampered frame must fail, got %v", err)
	}
}
