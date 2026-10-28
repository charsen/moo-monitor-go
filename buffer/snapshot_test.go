package buffer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotCompare(t *testing.T) {
	b := openBuffer(t, privateTemp(t), "", 0, 10)
	ctx := context.Background()
	event := testSnapshot()
	hash := mustRecord(t, b, event, testNow)
	snapshots, err := b.Snapshot(ctx)
	if err != nil || len(snapshots) != 1 {
		t.Fatal(err)
	}
	old := snapshots[0]
	mustRecord(t, b, event, testNow) // Same timestamp, different actual count/content.
	current, err := b.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if old.Timestamp != current[0].Timestamp || old.Digest == current[0].Digest {
		t.Fatal("same-ms content identity")
	}
	for _, decision := range []Decision{{Disposition: Confirm}, {Disposition: Quarantine, Reason: "invalid_record"}} {
		if applied, err := b.Compare(ctx, old, decision); err != nil || applied {
			t.Fatal("stale ack changed current", err)
		}
	}
	if r := getRecord(t, b, hash); r.Count != 2 {
		t.Fatal("stale ack lost count")
	}
	if applied, err := b.Compare(ctx, current[0], Decision{Disposition: Confirm}); err != nil || !applied {
		t.Fatal(err)
	}
	if getRecord(t, b, hash).Count != 2 {
		t.Fatal("open anchor deleted")
	}
	seedBucket(t, b, hash, "resolved")
	resolved, _ := b.Snapshot(ctx)
	mustRecord(t, b, event, testNow) // Recurrence changes bucket and count during transport.
	if applied, err := b.Compare(ctx, resolved[0], Decision{Disposition: Confirm}); err != nil || applied {
		t.Fatal("resolved old ack deleted recurrence", err)
	}
	seedBucket(t, b, hash, "deleted")
	deleted, err := b.Snapshot(ctx)
	if err != nil || len(deleted) != 0 {
		t.Fatal("deleted scanned", err)
	}
	if n, err := b.Count(ctx, "deleted"); err != nil || n != 1 {
		t.Fatal("deleted changed")
	}
}

func TestQuarantineRecovery(t *testing.T) {
	for _, stage := range []string{"rename:target", "rename:receipt", "remove:source", "conflict", "receipt-conflict"} {
		t.Run(stage, func(t *testing.T) {
			root := privateTemp(t)
			b := openBuffer(t, root, "", 0, 10)
			ctx := context.Background()
			hash := mustRecord(t, b, testSnapshot(), testNow)
			snapshots, _ := b.Snapshot(ctx)
			snap := snapshots[0]
			base := "cloud-rejected/" + hash + "-" + snap.Digest
			source, _ := os.ReadFile(filepath.Join(b.path, "open", hash+".yaml"))
			injected := errors.New("quarantine injected")
			b.fault = func(current string) error {
				if stage == "rename:target" && current == "rename:"+base+".yaml" {
					return injected
				}
				if stage == "rename:receipt" && current == "rename:"+base+".receipt.json" {
					return injected
				}
				if stage == "remove:source" && current == "remove:open/"+hash+".yaml" {
					return injected
				}
				return nil
			}
			if stage == "conflict" || stage == "receipt-conflict" {
				if err := ensureDirectory(b.root, "cloud-rejected"); err != nil {
					t.Fatal(err)
				}
				name := base + ".yaml"
				if stage == "receipt-conflict" {
					name = base + ".receipt.json"
				}
				if err := b.root.WriteFile(name, []byte("different"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if applied, err := b.Compare(ctx, snap, Decision{Disposition: Quarantine, Reason: "invalid_record"}); err == nil || applied {
				t.Fatal("expected partial quarantine/conflict")
			}
			kept, _ := os.ReadFile(filepath.Join(b.path, "open", hash+".yaml"))
			if !bytes.Equal(source, kept) {
				t.Fatal("source removed before complete quarantine")
			}
			if stage == "conflict" || stage == "receipt-conflict" {
				return
			}
			b.Close()
			reopened := openBuffer(t, root, "", 0, 10)
			if applied, err := reopened.Compare(ctx, snap, Decision{Disposition: Quarantine, Reason: "invalid_record"}); err != nil || !applied {
				t.Fatal("quarantine retry", err)
			}
			kept, _ = os.ReadFile(filepath.Join(reopened.path, base+".yaml"))
			if !bytes.Equal(source, kept) {
				t.Fatal("quarantine bytes changed")
			}
			data, err := os.ReadFile(filepath.Join(reopened.path, base+".receipt.json"))
			if err != nil {
				t.Fatal(err)
			}
			var receipt rejectionReceipt
			if err = json.Unmarshal(data, &receipt); err != nil || receipt.Reason != "invalid_record" || receipt.Retryable || receipt.Digest != snap.Digest {
				t.Fatal("durable rejection", err)
			}
			if record, err := reopened.Get(ctx, hash); err != nil || record != nil {
				t.Fatal("source remains after complete quarantine", err)
			}
		})
	}
}

func TestQuarantineReasonAndFailures(t *testing.T) {
	b := openBuffer(t, privateTemp(t), "", 0, 10)
	ctx := context.Background()
	hash := mustRecord(t, b, testSnapshot(), testNow)
	snapshots, _ := b.Snapshot(ctx)
	snap := snapshots[0]
	if applied, err := b.Compare(ctx, snap, Decision{Disposition: Quarantine, Reason: "token=fixed-fake-secret invalid_record"}); err != nil || !applied {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(b.path, "cloud-rejected", hash+"-"+snap.Digest+".receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("fixed-fake")) || !bytes.Contains(data, []byte(`"reason":"rejected"`)) {
		t.Fatal("raw reason leaked")
	}
	for _, reason := range []string{"INVALID_RECORD", "invalid_record\n", "new-code"} {
		if SafeRejectionReason(reason) != "rejected" {
			t.Fatal("unknown reason accepted")
		}
	}
}

func TestSnapshotTimestampAndResolved(t *testing.T) {
	b := openBuffer(t, privateTemp(t), "", 0, 10)
	ctx := context.Background()
	hash := mustRecord(t, b, testSnapshot(), testNow)
	seedBucket(t, b, hash, "resolved")
	snapshots, err := b.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := b.Compare(ctx, snapshots[0], Decision{Disposition: Confirm}); err != nil || !applied {
		t.Fatal("resolved confirmation", err)
	}
	hash = mustRecord(t, b, testSnapshot(), testNow.Add(time.Hour))
	record := getRecord(t, b, hash)
	record.Meta["updated_at"] = ""
	if err := b.writeRecord("open", *record); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Snapshot(ctx); err == nil {
		t.Fatal("empty timestamp silently fell back")
	}
}

func TestConfirmResolvedFailure(t *testing.T) {
	b := openBuffer(t, privateTemp(t), "", 0, 10)
	hash := mustRecord(t, b, testSnapshot(), testNow)
	seedBucket(t, b, hash, "resolved")
	snapshots, err := b.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("fixed resolved removal failure")
	b.fault = func(stage string) error {
		if stage == "remove:resolved/"+hash+".yaml" {
			return injected
		}
		return nil
	}
	if applied, err := b.Compare(context.Background(), snapshots[0], Decision{Disposition: Confirm}); applied || !errors.Is(err, injected) {
		t.Fatal("resolved removal failure", err)
	}
	if r := getRecord(t, b, hash); r.Status != "resolved" {
		t.Fatal("failed confirm removed source")
	}
	b.fault = nil
	if applied, err := b.Compare(context.Background(), snapshots[0], Decision{Disposition: Confirm}); !applied || err != nil {
		t.Fatal("confirm retry", err)
	}
}
