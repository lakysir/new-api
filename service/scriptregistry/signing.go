package scriptregistry

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
)

// Manifest is the signed metadata bound to an immutable script version. It
// mirrors MarketScriptManifest in the architecture doc §5.1. Signature and
// SignatureKeyId are excluded from the signing payload (a signature cannot sign
// itself); everything else is covered.
type Manifest struct {
	ScriptID       string   `json:"scriptId"`
	Version        string   `json:"version"`
	Title          string   `json:"title"`
	TaskType       string   `json:"taskType"`
	AllowedOrigins []string `json:"allowedOrigins"`
	ParamsSchema   string   `json:"paramsSchema"`
	ResultSchema   string   `json:"resultSchema,omitempty"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
	CodeSha256     string   `json:"codeSha256"`
	ReviewStatus   string   `json:"reviewStatus"`
	PublishedAt    int64    `json:"publishedAt"`
	SignatureKeyID string   `json:"signatureKeyId,omitempty"`
	Signature      string   `json:"signature,omitempty"`
}

var (
	// ErrNoSigningKey is returned when the platform signing key is unconfigured.
	ErrNoSigningKey = errors.New("script signing key is not configured")
	// ErrBadSignature is returned when a manifest fails signature verification.
	ErrBadSignature = errors.New("manifest signature is invalid")
)

// SigningPayload returns the canonical bytes that are signed and verified. It
// uses a deterministic JSON encoding with sorted keys and the signature/keyId
// fields cleared, so the same logical manifest always yields the same payload
// regardless of field order in transit.
func (m Manifest) SigningPayload() ([]byte, error) {
	c := m
	c.Signature = ""
	c.SignatureKeyID = ""
	sort.Strings(c.AllowedOrigins)
	return canonicalJSON(c)
}

// canonicalJSON marshals v with map keys sorted. encoding/json already sorts
// struct fields by declaration order deterministically and map keys
// lexicographically, so a plain Marshal is canonical for our fixed structs.
func canonicalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Signer produces Ed25519 signatures over manifests using the platform key.
type Signer struct {
	keyID string
	priv  ed25519.PrivateKey
}

// NewSigner builds a Signer from a base64-encoded Ed25519 seed or full private
// key. Empty seed yields ErrNoSigningKey so callers can degrade explicitly
// rather than sign with a zero key.
func NewSigner(keyID, base64Seed string) (*Signer, error) {
	if base64Seed == "" {
		return nil, ErrNoSigningKey
	}
	raw, err := base64.StdEncoding.DecodeString(base64Seed)
	if err != nil {
		return nil, err
	}
	var priv ed25519.PrivateKey
	switch len(raw) {
	case ed25519.SeedSize:
		priv = ed25519.NewKeyFromSeed(raw)
	case ed25519.PrivateKeySize:
		priv = ed25519.PrivateKey(raw)
	default:
		return nil, errors.New("invalid ed25519 key length")
	}
	return &Signer{keyID: keyID, priv: priv}, nil
}

// KeyID returns the signer's key identifier, stored on each signed version.
func (s *Signer) KeyID() string { return s.keyID }

// PublicKeyBase64 returns the base64 public key, for distribution to plugins.
func (s *Signer) PublicKeyBase64() string {
	pub := s.priv.Public().(ed25519.PublicKey)
	return base64.StdEncoding.EncodeToString(pub)
}

// Sign returns the base64 Ed25519 signature over the manifest signing payload
// and stamps the manifest with this signer's key id.
func (s *Signer) Sign(m *Manifest) (string, error) {
	m.SignatureKeyID = s.keyID
	payload, err := m.SigningPayload()
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(s.priv, payload)
	b64 := base64.StdEncoding.EncodeToString(sig)
	m.Signature = b64
	return b64, nil
}

// VerifyManifest checks a manifest's signature against the given base64 public
// key. It returns ErrBadSignature on any mismatch so callers never execute
// unverified code.
func VerifyManifest(m Manifest, base64PubKey string) error {
	pubRaw, err := base64.StdEncoding.DecodeString(base64PubKey)
	if err != nil {
		return err
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return ErrBadSignature
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return ErrBadSignature
	}
	payload, err := m.SigningPayload()
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(pubRaw), payload, sig) {
		return ErrBadSignature
	}
	return nil
}
