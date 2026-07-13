package model

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/service/nodeidentity"
)

// nowPlus returns a unix timestamp offset by delta seconds from now.
func nowPlus(deltaSeconds int64) int64 {
	return time.Now().Unix() + deltaSeconds
}

// activateTestDevice runs the full challenge/sign/activate handshake for a user.
func activateTestDevice(t *testing.T, userId int) (*Device, *DeviceSession, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	ch, err := CreateDeviceChallenge(userId)
	if err != nil {
		t.Fatal(err)
	}
	sig := signChallenge(priv, ch.Nonce)
	device, session, err := ActivateDevice(userId, ch.Id, pubB64, sig, "test-device")
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	return device, session, priv
}

// signChallenge mirrors nodeidentity.challengeMessage domain separation.
func signChallenge(priv ed25519.PrivateKey, nonce string) string {
	msg := []byte("ai-token-p2p:device-challenge:v1:" + nonce)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))
}

func TestDeviceActivateAndAuthenticate(t *testing.T) {
	device, session, _ := activateTestDevice(t, 501)
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatal("session must include access and refresh tokens")
	}
	got, err := AuthenticateDeviceAccessToken(device.Id, session.AccessToken)
	if err != nil {
		t.Fatalf("valid access token should authenticate: %v", err)
	}
	if got.Id != device.Id {
		t.Fatal("authenticated device id mismatch")
	}
	// Raw tokens must never be persisted; only hashes.
	if got.AccessTokenHash == session.AccessToken {
		t.Fatal("access token must be stored hashed, not in plaintext")
	}
}

func TestDeviceChallengeCannotReplay(t *testing.T) {
	userId := 502
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	ch, err := CreateDeviceChallenge(userId)
	if err != nil {
		t.Fatal(err)
	}
	sig := signChallenge(priv, ch.Nonce)
	if _, _, err := ActivateDevice(userId, ch.Id, pubB64, sig, "d1"); err != nil {
		t.Fatalf("first activation should succeed: %v", err)
	}
	// Reusing the same challenge must fail (one-time nonce).
	if _, _, err := ActivateDevice(userId, ch.Id, pubB64, sig, "d2"); err != ErrChallengeInvalid {
		t.Fatalf("replay must fail with ErrChallengeInvalid, got %v", err)
	}
}

func TestDeviceActivateRejectsBadSignature(t *testing.T) {
	userId := 503
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	ch, _ := CreateDeviceChallenge(userId)
	badSig := signChallenge(otherPriv, ch.Nonce) // signed by a different key
	if _, _, err := ActivateDevice(userId, ch.Id, pubB64, badSig, "d"); err != nodeidentity.ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestRevokedDeviceCannotRefreshOrAuth(t *testing.T) {
	userId := 504
	device, session, _ := activateTestDevice(t, userId)
	if err := RevokeDevice(userId, device.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := AuthenticateDeviceAccessToken(device.Id, session.AccessToken); err != ErrDeviceRevoked {
		t.Fatalf("revoked device must not authenticate, got %v", err)
	}
	if _, err := RefreshDeviceSession(device.Id, session.RefreshToken); err != ErrDeviceRevoked {
		t.Fatalf("revoked device must not refresh, got %v", err)
	}
}

func TestRefreshRotatesTokens(t *testing.T) {
	device, session, _ := activateTestDevice(t, 505)
	newSession, err := RefreshDeviceSession(device.Id, session.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if newSession.AccessToken == session.AccessToken {
		t.Fatal("refresh must rotate the access token")
	}
	// Old access token must no longer authenticate after rotation.
	if _, err := AuthenticateDeviceAccessToken(device.Id, session.AccessToken); err != ErrDeviceTokenInvalid {
		t.Fatalf("old access token must be invalid after refresh, got %v", err)
	}
}

// seedApprovedVersion publishes an executable script version for capability tests.
func seedApprovedVersion(t *testing.T, scriptId int) *ScriptVersion {
	t.Helper()
	v := newTestScriptVersion(scriptId)
	if err := CreateScriptVersion(v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEnableCapabilityRequiresExecutableVersion(t *testing.T) {
	cap := &NodeCapability{
		NodeId: "node_x", ScriptId: 8001, Version: 1, UserId: 1,
		TestExpiresAt: nowPlus(3600),
	}
	// No such version exists yet.
	if err := EnableCapability(cap); err != ErrScriptVersionNotFound {
		t.Fatalf("expected ErrScriptVersionNotFound, got %v", err)
	}
}

func TestEnableCapabilityRequiresValidTest(t *testing.T) {
	scriptId := 8002
	v := seedApprovedVersion(t, scriptId)
	cap := &NodeCapability{
		NodeId: "node_y", ScriptId: scriptId, Version: v.Version, UserId: 1,
		TestExpiresAt: nowPlus(-10), // expired
	}
	if err := EnableCapability(cap); err != ErrCapabilityTestRequired {
		t.Fatalf("expected ErrCapabilityTestRequired, got %v", err)
	}
}

func TestEnableCapabilitySucceedsAndRevocationSuspends(t *testing.T) {
	scriptId := 8003
	v := seedApprovedVersion(t, scriptId)
	cap := &NodeCapability{
		NodeId: "node_z", ScriptId: scriptId, Version: v.Version, UserId: 1,
		PriceMicros: 120000, DailyQuota: 100, TestExpiresAt: nowPlus(3600),
	}
	if err := EnableCapability(cap); err != nil {
		t.Fatalf("enable should succeed: %v", err)
	}
	if cap.Status != CapabilityStatusActive {
		t.Fatal("capability should be active")
	}
	// Revoking the script version must suspend the capability (N-007).
	if _, err := SuspendCapabilitiesByScriptVersion(scriptId, v.Version); err != nil {
		t.Fatal(err)
	}
	if err := RevokeScriptVersion(scriptId, v.Version, "sec", "critical"); err != nil {
		t.Fatal(err)
	}
	caps, _ := ListNodeCapabilities("node_z")
	if len(caps) != 1 || caps[0].Status != CapabilityStatusSuspended {
		t.Fatalf("capability should be suspended after revoke, got %+v", caps)
	}
}

func TestNodePresenceOnlineWindow(t *testing.T) {
	n := &Node{Id: "node_p", State: NodeStateIdle, LastSeenAt: nowPlus(0)}
	if !n.IsOnline() {
		t.Fatal("fresh heartbeat should be online")
	}
	n.LastSeenAt = nowPlus(-60) // older than 45s timeout
	if n.IsOnline() {
		t.Fatal("stale heartbeat should be offline")
	}
}

// activateWithKey runs the handshake with a caller-provided key pair so the
// same device public key can be reused across activations.
func activateWithKey(t *testing.T, userId int, pub string, priv ed25519.PrivateKey) (*Device, *DeviceSession) {
	t.Helper()
	ch, err := CreateDeviceChallenge(userId)
	if err != nil {
		t.Fatal(err)
	}
	sig := signChallenge(priv, ch.Nonce)
	device, session, err := ActivateDevice(userId, ch.Id, pub, sig, "d")
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	return device, session
}

func TestActivateIdempotentOnSameKey(t *testing.T) {
	userId := 601
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	d1, s1 := activateWithKey(t, userId, pubB64, priv)
	// Re-activate with the SAME device key (simulates repeat click / restart).
	d2, s2 := activateWithKey(t, userId, pubB64, priv)

	if d1.Id != d2.Id {
		t.Fatalf("same key must reuse the device row: %s vs %s", d1.Id, d2.Id)
	}
	// Tokens are reissued, so the old access token should be invalid now.
	if _, err := AuthenticateDeviceAccessToken(d1.Id, s1.AccessToken); err != ErrDeviceTokenInvalid {
		t.Fatalf("old token should be rotated out, got %v", err)
	}
	if _, err := AuthenticateDeviceAccessToken(d2.Id, s2.AccessToken); err != nil {
		t.Fatalf("new token should authenticate, got %v", err)
	}

	// Exactly one device row for this user+key.
	var count int64
	DB.Model(&Device{}).Where("user_id = ? AND public_key = ?", userId, pubB64).Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 device for repeated activation, got %d", count)
	}
}

func TestDifferentKeysAreDifferentDevices(t *testing.T) {
	userId := 602
	pubA, privA, _ := ed25519.GenerateKey(rand.Reader)
	pubB, privB, _ := ed25519.GenerateKey(rand.Reader)
	dA, _ := activateWithKey(t, userId, base64.StdEncoding.EncodeToString(pubA), privA)
	dB, _ := activateWithKey(t, userId, base64.StdEncoding.EncodeToString(pubB), privB)
	if dA.Id == dB.Id {
		t.Fatal("different device keys (e.g. two browsers) must be distinct devices")
	}
}

func TestAuthenticateDeviceByToken(t *testing.T) {
	device, session, _ := activateTestDevice(t, 610)
	got, err := AuthenticateDeviceByToken(session.AccessToken)
	if err != nil {
		t.Fatalf("valid device token should authenticate: %v", err)
	}
	if got.Id != device.Id || got.UserId != 610 {
		t.Fatalf("resolved wrong device/user: %+v", got)
	}
	if _, err := AuthenticateDeviceByToken("nope"); err != ErrDeviceTokenInvalid {
		t.Fatalf("bad token must fail, got %v", err)
	}
	if err := RevokeDevice(610, device.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := AuthenticateDeviceByToken(session.AccessToken); err == nil {
		t.Fatal("revoked device token must not authenticate")
	}
}
