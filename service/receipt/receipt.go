// Package receipt implements the control-plane side of the E2EE data plane's
// proof-of-execution: canonical receipt encoding, Ed25519 device-signature
// verification and result-hash reconciliation between the client and provider
// receipts (architecture §9.3). It holds no plaintext parameters or results —
// only hashes and signatures.
package receipt

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// Party identifies which side signed a receipt.
const (
	PartyProvider = "provider"
	PartyClient   = "client"
)

var (
	// ErrBadSignature is returned when a receipt signature does not verify.
	ErrBadSignature = errors.New("receipt signature invalid")
	// ErrBadPublicKey is returned for a malformed device public key.
	ErrBadPublicKey = errors.New("invalid device public key")
)

// Receipt is the signed proof both parties produce. The provider fills all
// execution fields; the client receipt echoes task_id/attempt and the
// result_hash it received. Signature and the signing key are excluded from the
// signed payload.
type Receipt struct {
	TaskId        string `json:"task_id"`
	Attempt       int    `json:"attempt"`
	Party         string `json:"party"`
	ScriptId      string `json:"script_id"`
	ScriptVersion string `json:"script_version"`
	CodeSha256    string `json:"code_sha256"`
	InputHash     string `json:"input_hash"`
	ResultHash    string `json:"result_hash"`
	StartedAt     string `json:"started_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
	DurationMs    int64  `json:"duration_ms"`
	Signature     string `json:"signature,omitempty"`
}

// SigningPayload returns the canonical bytes signed and verified: the receipt
// with the signature cleared, encoded deterministically. Go's encoding/json
// emits struct fields in declaration order, giving a stable canonical form both
// SDKs can reproduce.
func (r Receipt) SigningPayload() ([]byte, error) {
	c := r
	c.Signature = ""
	return json.Marshal(c)
}

// Verify checks the receipt's Ed25519 signature against a device public key.
func Verify(r Receipt, base64PubKey string) error {
	pubRaw, err := base64.StdEncoding.DecodeString(base64PubKey)
	if err != nil || len(pubRaw) != ed25519.PublicKeySize {
		return ErrBadPublicKey
	}
	sig, err := base64.StdEncoding.DecodeString(r.Signature)
	if err != nil {
		return ErrBadSignature
	}
	payload, err := r.SigningPayload()
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(pubRaw), payload, sig) {
		return ErrBadSignature
	}
	return nil
}

// Sign signs a receipt with a device private key (used by SDK/plugin; provided
// here so tests and tools can build valid vectors). Returns base64 signature.
func Sign(r *Receipt, priv ed25519.PrivateKey) (string, error) {
	payload, err := r.SigningPayload()
	if err != nil {
		return "", err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, payload))
	r.Signature = sig
	return sig, nil
}

// Reconciliation is the outcome of comparing the two parties' receipts.
type Reconciliation struct {
	Match      bool
	ResultHash string
	Reason     string
}

// Reconcile compares a provider and client receipt. They agree when both are
// for the same (task_id, attempt) and carry the same result_hash. A mismatch
// is not an error — it routes the order to dispute/sampling (architecture §9.3).
func Reconcile(provider, client Receipt) Reconciliation {
	if provider.TaskId != client.TaskId || provider.Attempt != client.Attempt {
		return Reconciliation{Match: false, Reason: "task/attempt mismatch"}
	}
	if provider.ResultHash == "" || client.ResultHash == "" {
		return Reconciliation{Match: false, Reason: "missing result hash"}
	}
	if provider.ResultHash != client.ResultHash {
		return Reconciliation{Match: false, ResultHash: provider.ResultHash, Reason: "result hash mismatch"}
	}
	return Reconciliation{Match: true, ResultHash: provider.ResultHash}
}
