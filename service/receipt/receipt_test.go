package receipt

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func sampleProviderReceipt() Receipt {
	return Receipt{
		TaskId: "tsk_1", Attempt: 1, Party: PartyProvider,
		ScriptId: "1", ScriptVersion: "3", CodeSha256: "sha256:abc",
		InputHash: "sha256:in", ResultHash: "sha256:out",
		StartedAt: "2026-07-12T00:00:00Z", CompletedAt: "2026-07-12T00:00:42Z",
		DurationMs: 42000,
	}
}

func TestReceiptSignVerifyRoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := sampleProviderReceipt()
	if _, err := Sign(&r, priv); err != nil {
		t.Fatal(err)
	}
	if err := Verify(r, base64.StdEncoding.EncodeToString(pub)); err != nil {
		t.Fatalf("valid receipt must verify: %v", err)
	}
}

func TestReceiptTamperFailsVerification(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := sampleProviderReceipt()
	_, _ = Sign(&r, priv)
	r.ResultHash = "sha256:tampered"
	if err := Verify(r, base64.StdEncoding.EncodeToString(pub)); err != ErrBadSignature {
		t.Fatalf("tampered receipt must fail, got %v", err)
	}
}

func TestReconcileMatch(t *testing.T) {
	p := sampleProviderReceipt()
	c := Receipt{TaskId: "tsk_1", Attempt: 1, Party: PartyClient, ResultHash: "sha256:out"}
	rec := Reconcile(p, c)
	if !rec.Match || rec.ResultHash != "sha256:out" {
		t.Fatalf("receipts with same hash must match: %+v", rec)
	}
}

func TestReconcileHashMismatch(t *testing.T) {
	p := sampleProviderReceipt()
	c := Receipt{TaskId: "tsk_1", Attempt: 1, Party: PartyClient, ResultHash: "sha256:different"}
	rec := Reconcile(p, c)
	if rec.Match || rec.Reason != "result hash mismatch" {
		t.Fatalf("mismatched hashes must not match: %+v", rec)
	}
}

func TestReconcileTaskMismatch(t *testing.T) {
	p := sampleProviderReceipt()
	c := Receipt{TaskId: "tsk_2", Attempt: 1, Party: PartyClient, ResultHash: "sha256:out"}
	if Reconcile(p, c).Match {
		t.Fatal("different task ids must not reconcile")
	}
}

// TestCanonicalPayloadVector pins the canonical signing payload so the
// TypeScript SDK can be tested against the same bytes (cross-language vector,
// architecture §22.5). If this changes, the shared fixture must change too.
func TestCanonicalPayloadVector(t *testing.T) {
	r := sampleProviderReceipt()
	payload, err := r.SigningPayload()
	if err != nil {
		t.Fatal(err)
	}
	got := string(payload)
	want := `{"task_id":"tsk_1","attempt":1,"party":"provider","script_id":"1","script_version":"3","code_sha256":"sha256:abc","input_hash":"sha256:in","result_hash":"sha256:out","started_at":"2026-07-12T00:00:00Z","completed_at":"2026-07-12T00:00:42Z","duration_ms":42000}`
	if got != want {
		t.Fatalf("canonical payload drift:\n got: %s\nwant: %s", got, want)
	}
	// Cross-check against the shared fixture committed for the SDK, if present.
	fixture := filepath.Join("..", "..", "..", "client-sdk", "protocol", "receipt_canonical_vector.json")
	if data, err := os.ReadFile(fixture); err == nil {
		if string(data) != want+"\n" && string(data) != want {
			t.Fatalf("fixture mismatch with canonical payload:\nfixture: %s\ncanon:   %s", string(data), want)
		}
	}
}
