package cloudsync

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
)

func pendingFiles(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	values := map[string][32]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		values[relative] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return values
}
func TestPendingStatsExactAndReadOnly(t *testing.T) {
	ctx := context.Background()
	store := makeStore(t, privateRoot(t), "pending")
	hashes := []string{add(t, store, 201, clock), add(t, store, 202, clock), add(t, store, 203, clock)}
	root, _, err := store.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(root, "open", hashes[2]+".yaml"), filepath.Join(root, "resolved", hashes[2]+".yaml")); err != nil {
		t.Fatal(err)
	}
	reader, err := NewStatusReader(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	snapshots, err := store.ReadOnlySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ledger := state{Version: 1, Cursor: clock.Format("2006-01-02T15:04:05.000Z07:00"), Confirmed: map[string]Confirmed{}}
	for _, snapshot := range snapshots {
		if snapshot.Hash == hashes[0] || snapshot.Hash == hashes[2] {
			ledger.Confirmed[snapshot.Hash] = Confirmed{Digest: snapshot.Digest, Timestamp: snapshot.Timestamp, Bucket: snapshot.Bucket}
		}
	}
	if err = reader.runtime.save(ledger); err != nil {
		t.Fatal(err)
	}
	before := pendingFiles(t, root)
	result, err := reader.Stats(ctx, PushBoth)
	if err != nil {
		t.Fatal(err)
	}
	want := PendingCounts{Scanned: 3, Open: 2, Resolved: 1, ConfirmedOpen: 1, ConfirmedResolved: 1, PendingOpen: 1}
	if !result.Runtimes.Known || result.Runtimes.Counts == nil || *result.Runtimes.Counts != want || result.Runtimes.Cursor == nil || *result.Runtimes.Cursor != ledger.Cursor {
		t.Fatal("exact counts/cursor", result)
	}
	if result.SlowSQL.Known || result.SlowSQL.Counts != nil || result.SlowSQL.Skipped != "source_unavailable" {
		t.Fatal("missing source became zero", result.SlowSQL)
	}
	if !reflect.DeepEqual(before, pendingFiles(t, root)) {
		t.Fatal("Stats wrote files")
	}
	// Same timestamp but changed count/bytes must invalidate the old ACK digest.
	add(t, store, 201, clock)
	before = pendingFiles(t, root)
	result, err = reader.Stats(ctx, PushRuntimes)
	if err != nil || !result.Runtimes.Known || result.Runtimes.Counts.PendingOpen != 2 || result.Runtimes.Counts.ConfirmedOpen != 0 {
		t.Fatal("same-millisecond change was lost", result, err)
	}
	if !reflect.DeepEqual(before, pendingFiles(t, root)) {
		t.Fatal("Stats pruned stale ACK/cursor")
	}
}
func TestPendingStatsUnknownAndJournal(t *testing.T) {
	for _, sample := range []struct{ name, file, content, reason string }{{"bad-state", stateFile, `{"version":1}`, "state"}, {"journal", "intent.yaml", "not: a valid journal\n", "recovery_required"}, {"bad-record", "record", "not: a valid record\n", "snapshot"}} {
		t.Run(sample.name, func(t *testing.T) {
			store := makeStore(t, privateRoot(t), "unknown")
			hash := add(t, store, 204, clock)
			reader, err := NewStatusReader(store, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			root, _, err := store.ScopeRoot()
			if err != nil {
				t.Fatal(err)
			}
			name := sample.file
			if name == "record" {
				name = filepath.Join("open", hash+".yaml")
			}
			if err = os.WriteFile(filepath.Join(root, name), []byte(sample.content), 0600); err != nil {
				t.Fatal(err)
			}
			before := pendingFiles(t, root)
			result, err := reader.Stats(context.Background(), PushRuntimes)
			if err == nil || result.Runtimes.Known || result.Runtimes.Counts != nil || result.Runtimes.Cursor != nil || result.Runtimes.Reason != sample.reason {
				t.Fatal("unknown became valid zero", result, err)
			}
			if sample.name == "journal" && !errors.Is(err, buffer.ErrRecoveryRequired) {
				t.Fatal("recovery requirement unidentifiable", err)
			}
			if !reflect.DeepEqual(before, pendingFiles(t, root)) {
				t.Fatal("read-only operation repaired state")
			}
		})
	}
}
func TestPendingStatsBusyAndCancel(t *testing.T) {
	store := makeStore(t, privateRoot(t), "busy-stats")
	add(t, store, 205, clock)
	entered, release := make(chan struct{}), make(chan struct{})
	syncer := makeSync(t, store, fakeClient(func(_ context.Context, batch []buffer.Snapshot) (Receipt, error) {
		close(entered)
		<-release
		return saved(batch), nil
	}))
	reader, err := NewStatusReader(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	done := make(chan error, 1)
	go func() { _, e := syncer.Sync(context.Background()); done <- e }()
	<-entered
	root, _, err := store.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	before := pendingFiles(t, root)
	for _, read := range []func() (PendingStats, error){func() (PendingStats, error) { return syncer.Stats(context.Background()) }, func() (PendingStats, error) {
		r, e := reader.Stats(context.Background(), PushRuntimes)
		return r.Runtimes.PendingStats, e
	}} {
		result, err := read()
		if err != nil || !result.Busy || result.Known || result.Counts != nil {
			t.Fatal("busy became zero", result, err)
		}
	}
	if !reflect.DeepEqual(before, pendingFiles(t, root)) {
		t.Fatal("busy stats wrote files")
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if r, e := reader.Stats(context.Background(), PushRuntimes); e != nil || !r.Runtimes.Known || r.Runtimes.Counts.ConfirmedOpen != 1 || r.Runtimes.Counts.PendingOpen != 0 {
		t.Fatal("real sync ACK not reflected", r, e)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := reader.Stats(canceled, PushRuntimes); !errors.Is(err, context.Canceled) || result.Runtimes.Known {
		t.Fatal("cancellation lost", err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	if result, err := reader.Stats(context.Background(), PushRuntimes); err == nil || result.Runtimes.Known || result.Runtimes.Counts != nil {
		t.Fatal("closed reader became zero", result, err)
	}
}
func TestPendingStatsSlowAndNoTransport(t *testing.T) {
	slow, input, now := slowSyncStore(t)
	if _, _, err := slow.ReportSlowQuery(context.Background(), input, now); err != nil {
		t.Fatal(err)
	}
	reader, err := NewStatusReader(nil, slow)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	result, err := reader.Stats(context.Background(), PushSlowSQL)
	if err != nil || !result.SlowSQL.Known || result.SlowSQL.Counts.PendingOpen != 1 {
		t.Fatal("slow pending", result, err)
	}
	store := makeStore(t, privateRoot(t), "pusher-stats")
	add(t, store, 206, clock)
	client := pushFixtureClient{runtime: func(context.Context, []buffer.Snapshot) (Receipt, error) {
		t.Fatal("Stats called transport")
		return Receipt{}, nil
	}}
	pusher, err := NewPusher(store, slow, client, PushConfig{Runtimes: true, SlowSQL: false, Timeout: time.Second, AfterPush: func(context.Context) error { t.Fatal("Stats called heartbeat"); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer pusher.Close(context.Background())
	result, err = pusher.Stats(context.Background(), PushBoth)
	if err != nil || !result.Runtimes.Known || result.Runtimes.Counts.PendingOpen != 1 || !result.SlowSQL.Known || result.SlowSQL.Counts.PendingOpen != 1 {
		t.Fatal("pusher pending", result, err)
	}
}
