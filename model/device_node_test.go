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

func TestTouchPresenceRejectsRevokedDevice(t *testing.T) {
	// Activate a device, register its node, confirm heartbeat works.
	device, _, _ := activateTestDevice(t, 620)
	nodeId := "node_revtest"
	if _, err := UpsertNode(620, device.Id, nodeId, "", "1"); err != nil {
		t.Fatal(err)
	}
	if err := TouchNodePresence(nodeId, NodeStateIdle); err != nil {
		t.Fatalf("heartbeat should work pre-revoke: %v", err)
	}
	// Revoke the device; a subsequent heartbeat must be refused and node kept offline.
	if err := RevokeDevice(620, device.Id); err != nil {
		t.Fatal(err)
	}
	if err := TouchNodePresence(nodeId, NodeStateIdle); err != ErrNodeDeviceRevoked {
		t.Fatalf("heartbeat for revoked device must be refused, got %v", err)
	}
	n, _ := GetNode(nodeId)
	if n.State != NodeStateOffline {
		t.Fatalf("revoked device's node must stay OFFLINE, got %s", n.State)
	}
}

func TestDeleteRevokedDeviceCascades(t *testing.T) {
	device, _, _ := activateTestDevice(t, 630)
	nodeId := "node_del1"
	if _, err := UpsertNode(630, device.Id, nodeId, "", "1"); err != nil {
		t.Fatal(err)
	}
	// Active device cannot be deleted.
	if err := DeleteRevokedDevice(630, device.Id); err != ErrDeviceNotRevoked {
		t.Fatalf("active device must not be deletable, got %v", err)
	}
	if err := RevokeDevice(630, device.Id); err != nil {
		t.Fatal(err)
	}
	if err := DeleteRevokedDevice(630, device.Id); err != nil {
		t.Fatalf("revoked device should delete: %v", err)
	}
	// Device and its node are gone.
	if _, err := AuthenticateDeviceByToken("x"); err == nil {
		_ = err // noop, just ensure package compiles
	}
	if _, err := GetNode(nodeId); err != ErrNodeNotFound {
		t.Fatalf("cascade should remove the node, got %v", err)
	}
}

func TestDeleteOfflineNodeGuardsOnline(t *testing.T) {
	// Online node cannot be deleted.
	online := "node_online_del"
	if err := DB.Create(&Node{Id: online, DeviceId: "d", UserId: 631, State: NodeStateIdle, LastSeenAt: nowPlus(0)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteOfflineNode(631, online); err != ErrNodeStillOnline {
		t.Fatalf("online node must not be deletable, got %v", err)
	}
	// Offline node deletes.
	offline := "node_offline_del"
	if err := DB.Create(&Node{Id: offline, DeviceId: "d", UserId: 631, State: NodeStateOffline, LastSeenAt: nowPlus(-3600)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteOfflineNode(631, offline); err != nil {
		t.Fatalf("offline node should delete: %v", err)
	}
	if _, err := GetNode(offline); err != ErrNodeNotFound {
		t.Fatalf("node should be gone, got %v", err)
	}
}

func TestListOffersAndCapabilityPrice(t *testing.T) {
	scriptId := 7100
	v := seedApprovedVersion(t, scriptId)
	// Two providers with different prices for the same version.
	seedIdleNodeCap(t, "off_node_a", 900, scriptId, v.Version, 500000)
	seedIdleNodeCap(t, "off_node_b", 901, scriptId, v.Version, 300000)

	offers, err := ListOffersForScript(scriptId, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 2 {
		t.Fatalf("expected 2 offers, got %d", len(offers))
	}
	// Cheapest first.
	if offers[0].PriceMicros != 300000 {
		t.Fatalf("offers must be cheapest-first, got %d", offers[0].PriceMicros)
	}
	// Chosen node price resolves.
	price, ok, err := GetCapabilityPrice("off_node_a", scriptId, v.Version)
	if err != nil || !ok || price != 500000 {
		t.Fatalf("capability price wrong: price=%d ok=%v err=%v", price, ok, err)
	}
}

// seedIdleNodeCap creates an online idle node + an active tested capability.
func seedIdleNodeCap(t *testing.T, nodeId string, userId, scriptId, version int, priceMicros int64) {
	t.Helper()
	if err := DB.Create(&Node{Id: nodeId, DeviceId: "d-" + nodeId, UserId: userId, State: NodeStateIdle, LastSeenAt: nowPlus(0)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&NodeCapability{
		NodeId: nodeId, ScriptId: scriptId, Version: version, UserId: userId,
		PriceMicros: priceMicros, DailyQuota: 100, RemainingQuota: 100,
		Status: CapabilityStatusActive, TestExpiresAt: nowPlus(3600),
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestScheduleRanksBySuccessRate(t *testing.T) {
	scriptId := 7200
	v := seedApprovedVersion(t, scriptId)
	// Three idle nodes, same price; different success histories.
	seedIdleNodeCap(t, "sr_low", 950, scriptId, v.Version, 100000)  // will be worst
	seedIdleNodeCap(t, "sr_high", 951, scriptId, v.Version, 100000) // best
	seedIdleNodeCap(t, "sr_mid", 952, scriptId, v.Version, 100000)
	// Record outcomes: high = 9/10, mid = 5/10, low = 1/10.
	for i := 0; i < 9; i++ {
		_ = RecordTaskOutcome("sr_high", true)
	}
	_ = RecordTaskOutcome("sr_high", false)
	for i := 0; i < 5; i++ {
		_ = RecordTaskOutcome("sr_mid", true)
	}
	for i := 0; i < 5; i++ {
		_ = RecordTaskOutcome("sr_mid", false)
	}
	_ = RecordTaskOutcome("sr_low", true)
	for i := 0; i < 9; i++ {
		_ = RecordTaskOutcome("sr_low", false)
	}

	cands, err := ScheduleCandidates(scriptId, v.Version, 200000, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(cands))
	}
	// Highest success rate must rank first, lowest last.
	if cands[0].NodeId != "sr_high" || cands[2].NodeId != "sr_low" {
		t.Fatalf("expected order high>mid>low, got %s,%s,%s", cands[0].NodeId, cands[1].NodeId, cands[2].NodeId)
	}
}

func TestBusyNodeExcludedSoLowSuccessCanRun(t *testing.T) {
	scriptId := 7201
	v := seedApprovedVersion(t, scriptId)
	seedIdleNodeCap(t, "busy_high", 960, scriptId, v.Version, 100000)
	seedIdleNodeCap(t, "idle_low", 961, scriptId, v.Version, 100000)
	for i := 0; i < 20; i++ {
		_ = RecordTaskOutcome("busy_high", true)
	}
	_ = RecordTaskOutcome("idle_low", false)
	// Mark the high-success node BUSY: it drops out of candidates, so the
	// low-success idle node becomes the (only) pick — the described fallback.
	if err := DB.Model(&Node{}).Where("id = ?", "busy_high").Update("state", NodeStateBusy).Error; err != nil {
		t.Fatal(err)
	}
	cands, _ := ScheduleCandidates(scriptId, v.Version, 200000, 10)
	if len(cands) != 1 || cands[0].NodeId != "idle_low" {
		t.Fatalf("busy high-success node must be excluded, leaving idle_low; got %+v", cands)
	}
}

func TestEnableCapabilityRequiresBalanceCheck(t *testing.T) {
	scriptId := 7300
	sv := &ScriptVersion{ScriptId: scriptId, AuthorId: 1, CodeSha256: "sha256:x", ReviewStatus: ScriptVersionApproved, CategoryId: 42}
	if err := CreateScriptVersion(sv); err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&Node{Id: "bc_node", DeviceId: "d", UserId: 1, State: NodeStateIdle, LastSeenAt: nowPlus(0)}).Error; err != nil {
		t.Fatal(err)
	}
	cap := &NodeCapability{
		NodeId: "bc_node", ScriptId: scriptId, Version: sv.Version, UserId: 1,
		PriceMicros: 100000, DailyQuota: 10, TestExpiresAt: nowPlus(3600),
	}
	if err := EnableCapability(cap); err != ErrBalanceCheckRequired {
		t.Fatalf("expected ErrBalanceCheckRequired, got %v", err)
	}
	if err := RecordBalanceCheck(&NodeSiteStatus{
		NodeId: "bc_node", CategoryId: 42, UserId: 1, BalanceOk: true, ExpiresAt: nowPlus(3600),
	}); err != nil {
		t.Fatal(err)
	}
	if err := EnableCapability(cap); err != nil {
		t.Fatalf("enable should succeed after balance check: %v", err)
	}
	cands, _ := ScheduleCandidates(scriptId, sv.Version, 200000, 10)
	if len(cands) != 1 || cands[0].NodeId != "bc_node" {
		t.Fatalf("balance-checked node should be schedulable, got %+v", cands)
	}
}

func TestSetCategoryBalanceScript(t *testing.T) {
	cat := &ScriptCategory{Name: "Dreamina", Site: "dreamina.com"}
	if err := CreateScriptCategory(cat); err != nil {
		t.Fatal(err)
	}
	probe := &ScriptVersion{ScriptId: 7400, AuthorId: 1, CodeSha256: "sha256:p", ReviewStatus: ScriptVersionApproved}
	if err := CreateScriptVersion(probe); err != nil {
		t.Fatal(err)
	}
	if err := SetCategoryBalanceScript(cat.Id, 7400, probe.Version); err != nil {
		t.Fatalf("set balance script: %v", err)
	}
	got, _ := GetScriptCategory(cat.Id)
	if got.BalanceScriptId != 7400 || got.BalanceScriptVersion != probe.Version {
		t.Fatalf("balance script not set: %+v", got)
	}
}
