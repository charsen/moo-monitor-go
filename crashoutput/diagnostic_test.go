package crashoutput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/cloudsync"
	"gitee.com/charsen/moo-monitor-go/records"
)

type diagnosticClient struct{ calls int }

func (c *diagnosticClient) Send(_ context.Context, batch []buffer.Snapshot) (cloudsync.Receipt, error) {
	c.calls++
	receipt := cloudsync.Receipt{Saved: int64(len(batch))}
	for i, item := range batch {
		receipt.Results = append(receipt.Results, cloudsync.Item{Index: i, Hash: item.Hash, Status: "saved"})
	}
	return receipt, nil
}
func TestDiagnosticPrepareApplyAndSync(t *testing.T) {
	source := t.TempDir()
	os.Chmod(source, 0700)
	root, file, name, err := createSpool(source)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	raw := []byte("panic: token=fixed-diagnostic-private\n")
	if _, err = file.Write(raw); err != nil {
		t.Fatal(err)
	}
	file.Close()
	manager, err := New(Config{Root: source, MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	targetRoot := t.TempDir()
	os.Chmod(targetRoot, 0700)
	builder, err := records.New(records.Config{MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := buffer.New(buffer.Config{Root: targetRoot, Scope: "crash-import"}, builder)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config := DiagnosticConfig{Env: "testing", Project: "diagnostic", GoVersion: "fixture-go", MaxReceipts: 1}
	at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	prepared, err := manager.Prepare(context.Background(), name[6:38], config, at)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.ArtifactID() != name[6:38] || len(prepared.RawDigest()) != 64 {
		t.Fatal("prepared identity")
	}
	result, err := manager.Apply(context.Background(), prepared, store)
	if err != nil || !result.Imported {
		t.Fatal("apply", err)
	}
	record, err := store.Get(context.Background(), result.Hash)
	if err != nil || record.Count != 1 || record.Exception.Class != "CrashDiagnostic" || record.Exception.File != "crash-output/"+prepared.ArtifactID() || record.Meta["diagnostic_may_be_partial"] != true || strings.Contains(record.Trace.Full, "fixed-diagnostic-private") {
		t.Fatal("unsafe diagnostic image", err)
	}
	client := &diagnosticClient{}
	syncer, err := cloudsync.New(store, client, cloudsync.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer syncer.Close()
	synced, err := syncer.Sync(context.Background())
	if err != nil || synced.Confirmed != 1 || synced.Pending != 0 || client.calls != 1 {
		t.Fatal("normal sync compatibility", err)
	}
	retry, err := manager.Prepare(context.Background(), name[6:38], config, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	result, err = manager.Apply(context.Background(), retry, store)
	if err != nil || !result.AlreadyImported {
		t.Fatal("retry duplicated event", err)
	}
	record, err = store.Get(context.Background(), result.Hash)
	if err != nil || record.Count != 1 || record.Context.OccurredAt != "2026-11-02T00:00:00.000+00:00" {
		t.Fatal("retry changed event", err)
	}
	after, err := os.ReadFile(filepath.Join(source, name))
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("raw modified", err)
	}
	if err = os.WriteFile(filepath.Join(source, name), append(raw, []byte("changed")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Apply(context.Background(), prepared, store); !errors.Is(err, ErrChanged) {
		t.Fatal("prepared version drift ignored", err)
	}
	changed, err := manager.Prepare(context.Background(), name[6:38], config, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Apply(context.Background(), changed, store); !errors.Is(err, buffer.ErrDiagnosticConflict) {
		t.Fatal("changed raw treated as already imported", err)
	}
}
