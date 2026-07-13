// Package nodeidentity implements Provider device identity primitives: device
// challenge/response using Ed25519 device keys, opaque platform token minting
// and verification. It is free of HTTP/GORM concerns so the security-critical
// signature and token logic can be unit tested in isolation (architecture
// §22.3, Stage C).
package nodeidentity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

var (
	// ErrBadPublicKey is returned for a malformed Ed25519 public key.
	ErrBadPublicKey = errors.New("invalid ed25519 public key")
	// ErrBadSignature is returned when a challenge signature does not verify.
	ErrBadSignature = errors.New("device challenge signature invalid")
)

// GenerateNonce returns a base64 random nonce of n bytes for device challenges.
func GenerateNonce(n int) (string, error) {
	if n <= 0 {
		n = 32
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// VerifyChallengeSignature verifies that the holder of the private key matching
// base64PubKey signed the given challenge nonce. The signed message binds the
// nonce with a fixed context string to prevent cross-protocol signature reuse.
func VerifyChallengeSignature(base64PubKey, nonce, base64Sig string) error {
	pubRaw, err := base64.StdEncoding.DecodeString(base64PubKey)
	if err != nil || len(pubRaw) != ed25519.PublicKeySize {
		return ErrBadPublicKey
	}
	sig, err := base64.StdEncoding.DecodeString(base64Sig)
	if err != nil {
		return ErrBadSignature
	}
	msg := challengeMessage(nonce)
	if !ed25519.Verify(ed25519.PublicKey(pubRaw), msg, sig) {
		return ErrBadSignature
	}
	return nil
}

// challengeMessage domain-separates the signed payload.
func challengeMessage(nonce string) []byte {
	return []byte("ai-token-p2p:device-challenge:v1:" + nonce)
}

// GenerateToken returns a random opaque token and its SHA-256 hash. Only the
// hash is persisted, so a database leak does not expose usable tokens.
func GenerateToken() (token string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	hash = HashToken(token)
	return token, hash, nil
}

// HashToken returns the hex SHA-256 of a token for storage/comparison.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ConstantTimeEqual compares two hex hashes without leaking timing.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
