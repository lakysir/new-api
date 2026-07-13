// Package dataplane implements the end-to-end encrypted data-plane crypto
// shared by the Client SDK and the Provider plugin (architecture §9). It uses
// X25519 ECDH, HKDF-SHA256 with a context that binds task_id/attempt/device
// ids/protocol version, and AES-256-GCM per direction. The relay never holds
// keys. This file is the Go reference; the TypeScript SDK reproduces it and
// both are validated against shared test vectors.
package dataplane

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

// ProtocolVersion is bound into the HKDF context so key material is
// domain-separated across protocol revisions.
const ProtocolVersion = "1.0"

// Direction identifies which side sends on a key. Keys are direction-specific
// so the two flows never share a keystream.
const (
	DirClientToProvider = "c2p"
	DirProviderToClient = "p2c"
)

var (
	// ErrKeyLen is returned for a malformed X25519 public key.
	ErrKeyLen = errors.New("invalid x25519 public key length")
	// ErrDecrypt is returned when AEAD open fails (tamper/wrong key/replay).
	ErrDecrypt = errors.New("aead open failed")
	// ErrShortFrame is returned when a frame is too short to parse.
	ErrShortFrame = errors.New("frame too short")
)

// SessionContext binds the derived keys to a specific task attempt and the two
// devices, preventing key reuse across tasks (architecture §9.1).
type SessionContext struct {
	TaskID           string
	Attempt          int
	ClientDeviceID   string
	ProviderDeviceID string
}

// GenerateKeyPair returns a fresh X25519 private/public key pair.
func GenerateKeyPair(rand io.Reader) (*ecdh.PrivateKey, error) {
	return ecdh.X25519().GenerateKey(rand)
}

// SharedSecret computes the raw X25519 shared secret from our private key and
// the peer's public key bytes.
func SharedSecret(priv *ecdh.PrivateKey, peerPub []byte) ([]byte, error) {
	pub, err := ecdh.X25519().NewPublicKey(peerPub)
	if err != nil {
		return nil, ErrKeyLen
	}
	return priv.ECDH(pub)
}

// hkdfInfo builds the HKDF info string binding the session context, direction
// and protocol version. Both sides must build byte-identical info.
func hkdfInfo(ctx SessionContext, direction string) []byte {
	// Fixed, unambiguous layout: field lengths are implied by the separators.
	s := "ai-token-p2p:dataplane:v" + ProtocolVersion +
		"|task=" + ctx.TaskID +
		"|attempt=" + itoa(ctx.Attempt) +
		"|client=" + ctx.ClientDeviceID +
		"|provider=" + ctx.ProviderDeviceID +
		"|dir=" + direction
	return []byte(s)
}

// DeriveKey derives a 32-byte AES-256 key for a direction from the shared
// secret and session context using HKDF-SHA256. The HKDF salt is empty (the
// shared secret is already high-entropy) and the context is carried in info.
func DeriveKey(sharedSecret []byte, ctx SessionContext, direction string) ([]byte, error) {
	r := hkdf.New(sha256.New, sharedSecret, nil, hkdfInfo(ctx, direction))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}

// Sealer encrypts outbound frames for one direction with a strictly increasing
// sequence number used as the AEAD nonce (deterministic, unique per key).
type Sealer struct {
	aead cipher.AEAD
	seq  uint64
}

// Opener decrypts inbound frames, rejecting out-of-order/replayed sequences.
type Opener struct {
	aead    cipher.AEAD
	nextSeq uint64
}

// NewSealer builds a Sealer from a 32-byte key.
func NewSealer(key []byte) (*Sealer, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// NewOpener builds an Opener from a 32-byte key.
func NewOpener(key []byte) (*Opener, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return &Opener{aead: aead}, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// nonceFromSeq builds a 12-byte GCM nonce from a sequence number (big-endian in
// the low 8 bytes). Unique per (key, seq) because seq strictly increases.
func nonceFromSeq(seq uint64) []byte {
	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce[4:], seq)
	return nonce
}

// Frame is the wire layout: an 8-byte big-endian sequence header followed by
// the AEAD ciphertext. The header is authenticated as additional data so it
// cannot be altered.
//
//	[ seq: 8 bytes big-endian ][ ciphertext+tag ]
func frameHeader(seq uint64) []byte {
	h := make([]byte, 8)
	binary.BigEndian.PutUint64(h, seq)
	return h
}

// Seal encrypts plaintext into the next frame, advancing the sequence.
func (s *Sealer) Seal(plaintext []byte) []byte {
	seq := s.seq
	s.seq++
	header := frameHeader(seq)
	ct := s.aead.Seal(nil, nonceFromSeq(seq), plaintext, header)
	return append(header, ct...)
}

// Open decrypts a frame, enforcing strictly increasing sequence numbers to
// reject replays and reordering.
func (o *Opener) Open(frame []byte) ([]byte, error) {
	if len(frame) < 8 {
		return nil, ErrShortFrame
	}
	header := frame[:8]
	seq := binary.BigEndian.Uint64(header)
	if seq != o.nextSeq {
		return nil, ErrDecrypt // out of order / replay
	}
	pt, err := o.aead.Open(nil, nonceFromSeq(seq), frame[8:], header)
	if err != nil {
		return nil, ErrDecrypt
	}
	o.nextSeq++
	return pt, nil
}

// itoa is a tiny int->string without importing strconv (keeps info building
// obviously allocation-light and matches the TS implementation's plain concat).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
