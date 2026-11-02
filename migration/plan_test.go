package migration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildImportPlanOffline(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "legacy")
	target := filepath.Join(base, "new-target")
	sourceFile := place(t, source, "open", "3e4258636dd8.yaml", fakeRecord(t, "runtimes"))
	cursor := filepath.Join(base, "cursor.json")
	if err := os.WriteFile(cursor, []byte(`{"runtimes":"2026-10-29T12:34:56+08:00"}`), 0600); err != nil {
		t.Fatal(err)
	}
	sources := Sources{RuntimeSource: source, CursorFile: cursor}
	options := Options{TargetRoot: target, Scope: "import-fixture", MaskKeys: []string{"password", "token"}}
	plan, err := BuildPlan(context.Background(), sources, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Records()) != 1 || plan.Digest == "" || plan.records[0].SourceDigest == "" || len(plan.artifacts) != 1 {
		t.Fatal("plan seal/records missing")
	}
	if _, err = os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("offline plan touched target", err)
	}
	second, err := BuildPlan(context.Background(), sources, options)
	if err != nil || second.Digest != plan.Digest {
		t.Fatal("plan not deterministic", err)
	}
	before, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "open", "unknown.yaml"), []byte("unsupported"), 0600); err != nil {
		t.Fatal(err)
	}
	if failed, err := BuildPlan(context.Background(), sources, options); err == nil || failed != nil {
		t.Fatal("bad source allowed partial plan")
	}
	after, err := os.ReadFile(sourceFile)
	if err != nil || string(after) != string(before) {
		t.Fatal("plan mutated source", err)
	}
	if _, err = BuildPlan(context.Background(), Sources{RuntimeSource: source}, Options{TargetRoot: source, Scope: "import-fixture"}); err == nil {
		t.Fatal("source and target overlap accepted")
	}
}

func TestBuildPlanInvalidUTF8Path(t *testing.T) {
	_, err := BuildPlan(context.Background(), Sources{RuntimeSource: "/fixed-source"}, Options{TargetRoot: "/fixed-" + string([]byte{255}), Scope: "imported"})
	if err == nil {
		t.Fatal("non-UTF8 path was silently normalized in the seal")
	}
}
