package model

import (
	"errors"
	"testing"
)

// migrateBindingTable ensures the bindings table exists in the shared in-memory
// test DB (TestMain in task_cas_test.go opens it).
func migrateBindingTable(t *testing.T) {
	t.Helper()
	if err := DB.AutoMigrate(&ScriptModelBinding{}); err != nil {
		t.Fatalf("migrate binding table: %v", err)
	}
}

func TestCreateScriptModelBinding_UniqueName(t *testing.T) {
	migrateBindingTable(t)
	b := &ScriptModelBinding{
		ModelName:       "veo-fast-test",
		ScriptId:        9001,
		Version:         1,
		PublisherUserId: 7,
	}
	if err := CreateScriptModelBinding(b); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if b.ConsumeMultiplier != 1 {
		t.Fatalf("expected default consume multiplier 1, got %d", b.ConsumeMultiplier)
	}

	dup := &ScriptModelBinding{ModelName: "veo-fast-test", ScriptId: 9002, Version: 1, PublisherUserId: 8}
	if err := CreateScriptModelBinding(dup); !errors.Is(err, ErrModelNameTaken) {
		t.Fatalf("want ErrModelNameTaken, got %v", err)
	}
}

func TestGetBindingByModelName(t *testing.T) {
	migrateBindingTable(t)
	b := &ScriptModelBinding{ModelName: "lookup-test", ScriptId: 9100, Version: 2, PublisherUserId: 3}
	if err := CreateScriptModelBinding(b); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := GetBindingByModelName("lookup-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ScriptId != 9100 || got.Version != 2 || got.PublisherUserId != 3 {
		t.Fatalf("unexpected binding: %+v", got)
	}
	if _, err := GetBindingByModelName("missing"); !errors.Is(err, ErrModelBindingNotFound) {
		t.Fatalf("want ErrModelBindingNotFound, got %v", err)
	}
}

func TestDeleteAndListBindings(t *testing.T) {
	migrateBindingTable(t)
	// Clean slate for the enabled-names assertion.
	DB.Where("1 = 1").Delete(&ScriptModelBinding{})

	names := []string{"m-a", "m-b"}
	for i, n := range names {
		if err := CreateScriptModelBinding(&ScriptModelBinding{
			ModelName: n, ScriptId: 9200 + i, Version: 1, PublisherUserId: 1, Enabled: true,
		}); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	enabled, err := ListEnabledBindingModelNames()
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 2 {
		t.Fatalf("expected 2 enabled names, got %v", enabled)
	}

	if err := DeleteScriptModelBindingByName("m-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := DeleteScriptModelBindingByName("m-a"); !errors.Is(err, ErrModelBindingNotFound) {
		t.Fatalf("second delete want ErrModelBindingNotFound, got %v", err)
	}
	all, err := ListScriptModelBindings()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 || all[0].ModelName != "m-b" {
		t.Fatalf("unexpected remaining bindings: %+v", all)
	}
}
