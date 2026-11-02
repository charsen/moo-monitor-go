package buffer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func diagnosticInput() DiagnosticInput {
	return DiagnosticInput{Identity: DiagnosticIdentity{ArtifactID: strings.Repeat("a", 32), RawDigest: strings.Repeat("b", 64)}, Snapshot: testSnapshot(), OccurredAt: testNow, MaxReceipts: 1}
}
func TestDiagnosticReceiptAfterRemoval(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0700)
	store := openBuffer(t, root, "diagnostic", 0, 500)
	input := diagnosticInput()
	first, err := store.ImportDiagnostic(context.Background(), input)
	if err != nil || !first.Imported || first.AlreadyImported {
		t.Fatal("initial import", first, err)
	}
	record := getRecord(t, store, first.Hash)
	if record.Count != 1 || record.Daily.Count != 1 || record.Exception.Class != "CrashDiagnostic" || record.Exception.File != "crash-output/"+input.Identity.ArtifactID {
		t.Fatal("incorrect event image")
	}
	record.Status = "resolved"
	if err = store.writeRecord("resolved", *record); err != nil {
		t.Fatal(err)
	}
	if err = store.remove("open/" + first.Hash + ".yaml"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 {
		t.Fatal(err)
	}
	if applied, err := store.Compare(context.Background(), snapshots[0], Decision{Disposition: Confirm}); err != nil || !applied {
		t.Fatal("confirmed removal", err)
	}
	input.OccurredAt = input.OccurredAt.Add(time.Hour)
	second, err := store.ImportDiagnostic(context.Background(), input)
	if err != nil || !second.AlreadyImported || second.Imported || second.Hash != first.Hash {
		t.Fatal("permanent receipt not honored", second, err)
	}
	if snapshots, err = store.Snapshot(context.Background()); err != nil || len(snapshots) != 0 {
		t.Fatal("confirmed event resurrected", err)
	}
	changed := input
	changed.Identity.RawDigest = strings.Repeat("c", 64)
	if _, err = store.ImportDiagnostic(context.Background(), changed); !errors.Is(err, ErrDiagnosticConflict) {
		t.Fatal("changed artifact accepted", err)
	}
	changed = input
	changed.Identity.ArtifactID = strings.Repeat("d", 32)
	if _, err = store.ImportDiagnostic(context.Background(), changed); !errors.Is(err, ErrReceiptLimit) {
		t.Fatal("new receipt exceeded budget", err)
	}
}
func TestDiagnosticFixedImageRecovery(t *testing.T) {
	for _, stage := range []string{"diagnostic-intent-durable", "diagnostic-record-durable", "diagnostic-receipt-durable"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			os.Chmod(root, 0700)
			store := openBuffer(t, root, "recover-diagnostic", 0, 500)
			sentinel := errors.New("fixed file fault")
			store.fault = func(actual string) error {
				if actual == stage {
					return sentinel
				}
				return nil
			}
			first, err := store.ImportDiagnostic(context.Background(), diagnosticInput())
			if !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, sentinel) || first.Imported {
				t.Fatal("uncertain result hidden", err)
			}
			if _, err = os.Stat(filepath.Join(store.path, "intent.yaml")); err != nil {
				t.Fatal("intent missing", err)
			}
			store.fault = nil
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := openBuffer(t, root, "recover-diagnostic", 0, 500)
			retry := diagnosticInput()
			retry.OccurredAt = retry.OccurredAt.Add(time.Hour)
			result, err := reopened.ImportDiagnostic(context.Background(), retry)
			if err != nil || !result.AlreadyImported {
				t.Fatal("fixed image did not resume", result, err)
			}
			record := getRecord(t, reopened, result.Hash)
			if record.Count != 1 || record.LastSeen != iso(testNow) {
				t.Fatal("recovery rebuilt or incremented event")
			}
		})
	}
}
func TestDiagnosticCapacityBeforeIntent(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0700)
	store := openBuffer(t, root, "full-diagnostic", 0, 0)
	if _, err := store.ImportDiagnostic(context.Background(), diagnosticInput()); !errors.Is(err, ErrFull) {
		t.Fatal("capacity bypassed", err)
	}
	for _, name := range []string{"intent.yaml", receiptDirectory} {
		if _, err := os.Stat(filepath.Join(store.path, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed import wrote ownership", name, err)
		}
	}
}

func TestDiagnosticRejectsForeignRecordAndBadReceipt(t *testing.T) {
	for _, mode := range []string{"foreign-record", "bad-json", "duplicate-key", "different-raw", "null"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			os.Chmod(root, 0700)
			store := openBuffer(t, root, "bad-diagnostic", 0, 500)
			input := diagnosticInput()
			if mode == "foreign-record" {
				snapshot := testSnapshot()
				snapshot.Exception.Class = "CrashDiagnostic"
				snapshot.Exception.Message = "Unverified crash diagnostic"
				snapshot.Exception.File = "crash-output/" + input.Identity.ArtifactID
				snapshot.Exception.Line = 0
				hash := mustRecord(t, store, snapshot, testNow)
				if _, err := store.ImportDiagnostic(context.Background(), input); !errors.Is(err, ErrDiagnosticConflict) {
					t.Fatal("existing event adopted", err)
				}
				if record := getRecord(t, store, hash); record.Count != 1 {
					t.Fatal("foreign event modified")
				}
				return
			}
			if _, err := store.ImportDiagnostic(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(store.path, receiptDirectory, input.Identity.ArtifactID+".json")
			original, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			changed := []byte("{")
			switch mode {
			case "duplicate-key":
				changed = append([]byte(`{"version":1,`), original[1:]...)
			case "different-raw":
				changed = []byte(strings.Replace(string(original), strings.Repeat("b", 64), strings.Repeat("c", 64), 1))
			case "null":
				changed = []byte(strings.Replace(string(original), `"version":1`, `"version":null`, 1))
			}
			if err = os.WriteFile(name, changed, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = store.ImportDiagnostic(context.Background(), input); !errors.Is(err, ErrDiagnosticConflict) {
				t.Fatal("bad receipt treated as absent/done", err)
			}
			after, err := os.ReadFile(name)
			if err != nil || string(after) != string(changed) {
				t.Fatal("bad receipt replaced", err)
			}
		})
	}
}
func TestDiagnosticSlowRejectsVersion2(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0700)
	store := openBuffer(t, root, "runtime-diagnostic", 0, 500)
	sentinel := errors.New("fixed prepare stop")
	store.fault = func(stage string) error {
		if stage == "diagnostic-intent-durable" {
			return sentinel
		}
		return nil
	}
	if _, err := store.ImportDiagnostic(context.Background(), diagnosticInput()); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(store.path, "intent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	slow := slowTestBuffer(t, root, "sql-diagnostic", 0, 500)
	name := filepath.Join(slow.path, "intent.yaml")
	if err = os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = slow.Snapshot(context.Background()); !errors.Is(err, ErrDiagnosticConflict) {
		t.Fatal("Slow accepted runtime intent", err)
	}
	after, err := os.ReadFile(name)
	if err != nil || string(after) != string(data) {
		t.Fatal("Slow changed intent", err)
	}
	store.fault = nil
	bad := []byte(strings.Replace(string(data), "image_digest:", "unrecognized:", 1))
	if err = os.WriteFile(filepath.Join(store.path, "intent.yaml"), bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Snapshot(context.Background()); !errors.Is(err, ErrDiagnosticConflict) {
		t.Fatal("bad v2 intent accepted", err)
	}
	after, err = os.ReadFile(filepath.Join(store.path, "intent.yaml"))
	if err != nil || string(after) != string(bad) {
		t.Fatal("bad intent changed", err)
	}
}

func TestDiagnosticRecoveryPreservesUnrelatedJournal(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0700)
	store := openBuffer(t, root, "conflicting-journals", 0, 500)
	store.fault = func(stage string) error {
		if stage == "diagnostic-intent-durable" {
			return errors.New("fixed stop")
		}
		return nil
	}
	if _, err := store.ImportDiagnostic(context.Background(), diagnosticInput()); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal(err)
	}
	store.fault = nil
	foreign := []byte("unrelated import ownership")
	if err := os.WriteFile(filepath.Join(store.path, ImportJournalName), foreign, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Snapshot(context.Background()); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("unrelated journal bypassed", err)
	}
	if rows, err := store.count("open"); err != nil || rows != 0 {
		t.Fatal("recovery wrote despite foreign journal", err)
	}
	data, err := os.ReadFile(filepath.Join(store.path, ImportJournalName))
	if err != nil || string(data) != string(foreign) {
		t.Fatal("foreign journal modified", err)
	}
}
