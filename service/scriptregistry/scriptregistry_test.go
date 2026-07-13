package scriptregistry

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

const sampleScript = `async function runGeneratedTest(config) {
  return { ok: true, echo: config };
}`

func TestNormalizeCodeIsStableAcrossLineEndings(t *testing.T) {
	crlf := strings.ReplaceAll(sampleScript, "\n", "\r\n")
	if NormalizeCode(sampleScript) != NormalizeCode(crlf) {
		t.Fatal("CRLF and LF variants must normalize identically")
	}
	if CodeSha256(NormalizeCode(sampleScript)) != CodeSha256(NormalizeCode(crlf)) {
		t.Fatal("hash must be stable across line endings")
	}
}

func TestHashChangesOnSingleByte(t *testing.T) {
	a := CodeSha256(NormalizeCode(sampleScript))
	b := CodeSha256(NormalizeCode(sampleScript + " "))
	if a == b {
		t.Fatal("one byte change must produce a different hash")
	}
}

func TestValidatePublishableRejectsMissingEntry(t *testing.T) {
	_, _, _, err := ValidatePublishable("const x = 1;")
	if err != ErrMissingEntry {
		t.Fatalf("expected ErrMissingEntry, got %v", err)
	}
}

func TestValidatePublishableRejectsEmpty(t *testing.T) {
	if _, _, _, err := ValidatePublishable("   \n  "); err != ErrEmptyCode {
		t.Fatalf("expected ErrEmptyCode, got %v", err)
	}
}

func TestValidatePublishableAcceptsArrowEntry(t *testing.T) {
	code := `const runGeneratedTest = async (config) => ({ ok: true });`
	_, _, _, err := ValidatePublishable(code)
	if err != nil {
		t.Fatalf("arrow-function entry should be accepted, got %v", err)
	}
}

func TestScanFlagsDangerousPrimitives(t *testing.T) {
	code := sampleScript + "\neval('x'); document.cookie;"
	findings := ScanCode(code)
	rules := map[string]bool{}
	for _, f := range findings {
		rules[f.Rule] = true
	}
	if !rules["eval"] || !rules["document_cookie"] {
		t.Fatalf("expected eval and document_cookie findings, got %+v", findings)
	}
}

func TestCleanScriptHasNoFindings(t *testing.T) {
	if f := ScanCode(NormalizeCode(sampleScript)); len(f) != 0 {
		t.Fatalf("clean script should have no findings, got %+v", f)
	}
}

func newTestSigner(t *testing.T) *Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	seed := base64.StdEncoding.EncodeToString(priv.Seed())
	s, err := NewSigner("test-key-1", seed)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	s := newTestSigner(t)
	m := Manifest{
		ScriptID: "scr_1", Version: "1", Title: "t", TaskType: "demo",
		AllowedOrigins: []string{"https://b.com", "https://a.com"},
		ParamsSchema:   `{"type":"object"}`, TimeoutSeconds: 60,
		CodeSha256: CodeSha256(NormalizeCode(sampleScript)), ReviewStatus: "approved",
		PublishedAt: 1,
	}
	if _, err := s.Sign(&m); err != nil {
		t.Fatal(err)
	}
	if m.SignatureKeyID != "test-key-1" {
		t.Fatal("key id must be stamped on manifest")
	}
	if err := VerifyManifest(m, s.PublicKeyBase64()); err != nil {
		t.Fatalf("valid signature should verify, got %v", err)
	}
}

func TestVerifyFailsOnTamper(t *testing.T) {
	s := newTestSigner(t)
	m := Manifest{ScriptID: "scr_1", Version: "1", CodeSha256: "sha256:abc"}
	if _, err := s.Sign(&m); err != nil {
		t.Fatal(err)
	}
	m.CodeSha256 = "sha256:def" // tamper after signing
	if err := VerifyManifest(m, s.PublicKeyBase64()); err != ErrBadSignature {
		t.Fatalf("tampered manifest must fail, got %v", err)
	}
}

func TestVerifyIgnoresOriginOrder(t *testing.T) {
	s := newTestSigner(t)
	m := Manifest{ScriptID: "scr_1", Version: "1", AllowedOrigins: []string{"https://a.com", "https://b.com"}}
	if _, err := s.Sign(&m); err != nil {
		t.Fatal(err)
	}
	m.AllowedOrigins = []string{"https://b.com", "https://a.com"} // reorder
	if err := VerifyManifest(m, s.PublicKeyBase64()); err != nil {
		t.Fatalf("origin reordering must not break verification, got %v", err)
	}
}

func TestNewSignerRejectsEmpty(t *testing.T) {
	if _, err := NewSigner("k", ""); err != ErrNoSigningKey {
		t.Fatalf("expected ErrNoSigningKey, got %v", err)
	}
}
