package model

import (
	"sync"
	"testing"
)

func newTestScriptVersion(scriptId int) *ScriptVersion {
	return &ScriptVersion{
		ScriptId:     scriptId,
		AuthorId:     1,
		Title:        "t",
		CodeSha256:   "sha256:abc",
		Code:         "async function runGeneratedTest(config){return {}}",
		ReviewStatus: ScriptVersionApproved,
	}
}

func TestCreateScriptVersionAssignsSequentialNumbers(t *testing.T) {
	scriptId := 1001
	for i := 1; i <= 3; i++ {
		v := newTestScriptVersion(scriptId)
		if err := CreateScriptVersion(v); err != nil {
			t.Fatalf("create v%d: %v", i, err)
		}
		if v.Version != i {
			t.Fatalf("expected version %d, got %d", i, v.Version)
		}
	}
}

func TestCreateScriptVersionConcurrentUnique(t *testing.T) {
	scriptId := 1002
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- CreateScriptVersion(newTestScriptVersion(scriptId))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create failed: %v", err)
		}
	}
	versions, err := ListScriptVersions(scriptId)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != n {
		t.Fatalf("expected %d unique versions, got %d", n, len(versions))
	}
	seen := map[int]bool{}
	for _, v := range versions {
		if seen[v.Version] {
			t.Fatalf("duplicate version number %d", v.Version)
		}
		seen[v.Version] = true
	}
}

func TestGetExecutableRejectsRevoked(t *testing.T) {
	scriptId := 1003
	v := newTestScriptVersion(scriptId)
	if err := CreateScriptVersion(v); err != nil {
		t.Fatal(err)
	}
	if _, err := GetExecutableScriptVersion(scriptId, v.Version); err != nil {
		t.Fatalf("fresh version should be executable: %v", err)
	}
	if err := RevokeScriptVersion(scriptId, v.Version, "security", "critical"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetExecutableScriptVersion(scriptId, v.Version); err != ErrScriptVersionRevoked {
		t.Fatalf("expected ErrScriptVersionRevoked, got %v", err)
	}
}

func TestRevokePreservesTimestampAndCode(t *testing.T) {
	scriptId := 1004
	v := newTestScriptVersion(scriptId)
	if err := CreateScriptVersion(v); err != nil {
		t.Fatal(err)
	}
	if err := RevokeScriptVersion(scriptId, v.Version, "first", "normal"); err != nil {
		t.Fatal(err)
	}
	first, err := GetScriptVersion(scriptId, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	firstTs := first.RevokedAt
	// Re-revoke: timestamp must not change, code must remain intact.
	if err := RevokeScriptVersion(scriptId, v.Version, "second", "critical"); err != nil {
		t.Fatal(err)
	}
	second, err := GetScriptVersion(scriptId, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if second.RevokedAt != firstTs {
		t.Fatalf("revoke timestamp changed on re-revoke: %d -> %d", firstTs, second.RevokedAt)
	}
	if second.Code != v.Code {
		t.Fatal("revocation must not mutate frozen code")
	}
	if second.RevokedReason != "second" {
		t.Fatal("reason should update on re-revoke")
	}
}

func TestGetScriptVersionNotFound(t *testing.T) {
	if _, err := GetScriptVersion(999999, 1); err != ErrScriptVersionNotFound {
		t.Fatalf("expected ErrScriptVersionNotFound, got %v", err)
	}
}

func TestDeleteHistoricalScriptVersionProtectsLatest(t *testing.T) {
	scriptId := 1005
	first := newTestScriptVersion(scriptId)
	second := newTestScriptVersion(scriptId)
	if err := CreateScriptVersion(first); err != nil { t.Fatal(err) }
	if err := CreateScriptVersion(second); err != nil { t.Fatal(err) }
	if err := DeleteHistoricalScriptVersion(scriptId, second.Version); err != ErrLatestScriptVersion {
		t.Fatalf("expected latest-version protection, got %v", err)
	}
	if err := DeleteHistoricalScriptVersion(scriptId, first.Version); err != nil { t.Fatal(err) }
	if _, err := GetScriptVersion(scriptId, first.Version); err != ErrScriptVersionNotFound {
		t.Fatalf("historical version still exists: %v", err)
	}
}

func TestUpdateScriptVersionPricingRebindsTemplate(t *testing.T) {
	scriptId := 1006
	v := newTestScriptVersion(scriptId)
	if err := CreateScriptVersion(v); err != nil { t.Fatal(err) }
	updated, err := UpdateScriptVersionPricing(scriptId, v.Version, 30_000, 80_000)
	if err != nil { t.Fatal(err) }
	if updated.PricingTemplateId == 0 { t.Fatal("pricing template was not bound") }
	tpl, err := GetPricingTemplate(updated.PricingTemplateId)
	if err != nil { t.Fatal(err) }
	if tpl.AuthorShareRatePPM != 30_000 || tpl.PlatformFeeRatePPM != 80_000 || tpl.PlatformFeeMinMicros != 0 {
		t.Fatalf("unexpected pricing template: %+v", tpl)
	}
}
