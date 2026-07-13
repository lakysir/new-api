package nodeidentity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func genKey(t *testing.T) (pub string, priv ed25519.PrivateKey) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pubKey), privKey
}

func TestVerifyChallengeSignatureRoundTrip(t *testing.T) {
	pub, priv := genKey(t)
	nonce, err := GenerateNonce(32)
	if err != nil {
		t.Fatal(err)
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, challengeMessage(nonce)))
	if err := VerifyChallengeSignature(pub, nonce, sig); err != nil {
		t.Fatalf("valid signature should verify: %v", err)
	}
}

func TestVerifyChallengeRejectsWrongNonce(t *testing.T) {
	pub, priv := genKey(t)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, challengeMessage("nonce-a")))
	if err := VerifyChallengeSignature(pub, "nonce-b", sig); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestVerifyChallengeRejectsWrongKey(t *testing.T) {
	pub, _ := genKey(t)
	_, otherPriv := genKey(t)
	nonce, _ := GenerateNonce(32)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(otherPriv, challengeMessage(nonce)))
	if err := VerifyChallengeSignature(pub, nonce, sig); err != ErrBadSignature {
		t.Fatalf("signature by another key must fail, got %v", err)
	}
}

func TestVerifyChallengeRejectsMalformedKey(t *testing.T) {
	if err := VerifyChallengeSignature("not-base64!!", "n", "c"); err != ErrBadPublicKey {
		t.Fatalf("expected ErrBadPublicKey, got %v", err)
	}
}

func TestGenerateTokenHashStable(t *testing.T) {
	tok, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if HashToken(tok) != hash {
		t.Fatal("HashToken must reproduce the stored hash")
	}
	if !ConstantTimeEqual(hash, HashToken(tok)) {
		t.Fatal("ConstantTimeEqual should match equal hashes")
	}
	if ConstantTimeEqual(hash, HashToken(tok+"x")) {
		t.Fatal("different tokens must not match")
	}
}

func TestNonceIsRandom(t *testing.T) {
	a, _ := GenerateNonce(32)
	b, _ := GenerateNonce(32)
	if a == b {
		t.Fatal("nonces must differ")
	}
}
