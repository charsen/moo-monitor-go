package cloudsync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/records"
)

func TestPruneExactResolvedAndDryRun(t *testing.T) {
	ctx := context.Background()
	store := makeStore(t, privateRoot(t), "prune")
	hashes := []string{add(t, store, 801, clock), add(t, store, 802, clock), add(t, store, 803, clock), add(t, store, 804, clock)}
	root, _, err := store.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, hash := range hashes[:3] {
		if err = os.Rename(filepath.Join(root, "open", hash+".yaml"), filepath.Join(root, "resolved", hash+".yaml")); err != nil {
			t.Fatal(err)
		}
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
	ledger := state{Version: 1, Cursor: clock.AddDate(10, 0, 0).Format("2006-01-02T15:04:05.000Z07:00"), Confirmed: map[string]Confirmed{}}
	for _, snapshot := range snapshots {
		if snapshot.Hash == hashes[0] || snapshot.Hash == hashes[2] || snapshot.Hash == hashes[3] {
			ledger.Confirmed[snapshot.Hash] = Confirmed{Digest: snapshot.Digest, Timestamp: snapshot.Timestamp, Bucket: snapshot.Bucket}
		}
	}
	if err = reader.runtime.save(ledger); err != nil {
		t.Fatal(err)
	}
	// A newer same-ms version invalidates the old ACK even if cursor is far ahead.
	add(t, store, 803, clock)
	if err = os.Rename(filepath.Join(root, "open", hashes[2]+".yaml"), filepath.Join(root, "resolved", hashes[2]+".yaml")); err != nil {
		t.Fatal(err)
	}
	// Non-active buckets are never maintenance candidates, even with matching names.
	protected, err := os.ReadFile(filepath.Join(root, "open", hashes[3]+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bucket := range []string{"deleted", "cloud-rejected"} {
		if err = os.MkdirAll(filepath.Join(root, bucket), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, bucket, hashes[3]+".yaml"), protected, 0600); err != nil {
			t.Fatal(err)
		}
	}
	before := pendingFiles(t, root)
	result, err := reader.Prune(ctx, PushRuntimes, PruneOptions{RetentionDays: 7, DryRun: true})
	if err != nil || !result.Runtimes.Known || result.Runtimes.Candidates != 1 || result.Runtimes.Purged != 0 {
		t.Fatal(result, err)
	}
	if !reflect.DeepEqual(before, pendingFiles(t, root)) {
		t.Fatal("dry maintenance wrote files")
	}
	result, err = reader.Prune(ctx, PushRuntimes, PruneOptions{RetentionDays: 1})
	if err != nil || result.Runtimes.Purged != 1 || result.Runtimes.Failed != 0 {
		t.Fatal(result, err)
	}
	after := pendingFiles(t, root)
	delete(before, filepath.Join("resolved", hashes[0]+".yaml"))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("maintenance changed protected record/ACK/other file")
	}
	result, err = reader.Prune(ctx, PushRuntimes, PruneOptions{RetentionDays: 99})
	if err != nil || result.Runtimes.Candidates != 0 {
		t.Fatal("maintenance not idempotent", result, err)
	}
}

type pruneIntercept struct {
	*buffer.Buffer
	before  func()
	failure error
}

func (p pruneIntercept) PruneResolved(ctx context.Context, snapshot buffer.Snapshot) (bool, error) {
	if p.before != nil {
		p.before()
	}
	if p.failure != nil {
		return false, p.failure
	}
	return p.Buffer.PruneResolved(ctx, snapshot)
}

func TestPruneRaceAndFailure(t *testing.T) {
	for _, mode := range []string{"changed", "failure", "journal"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			store := makeStore(t, privateRoot(t), "prune-race")
			hash := add(t, store, 811, clock)
			root, _, err := store.ScopeRoot()
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(filepath.Join(root, "open", hash+".yaml"), filepath.Join(root, "resolved", hash+".yaml")); err != nil {
				t.Fatal(err)
			}
			syncer, err := newStateAccess[records.Record](store, stateFile, lockFile)
			if err != nil {
				t.Fatal(err)
			}
			defer syncer.Close()
			snapshots, err := store.ReadOnlySnapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := snapshots[0]
			if err = syncer.save(state{Version: 1, Confirmed: map[string]Confirmed{hash: {Digest: snapshot.Digest, Timestamp: snapshot.Timestamp, Bucket: snapshot.Bucket}}}); err != nil {
				t.Fatal(err)
			}
			sentinel := errors.New("fixed prune failure")
			wrapped := pruneIntercept{Buffer: store}
			switch mode {
			case "changed":
				wrapped.before = func() {
					add(t, store, 811, clock)
					if err := os.Rename(filepath.Join(root, "open", hash+".yaml"), filepath.Join(root, "resolved", hash+".yaml")); err != nil {
						t.Fatal(err)
					}
				}
			case "failure":
				wrapped.failure = sentinel
			case "journal":
				wrapped.before = func() {
					if err := os.WriteFile(filepath.Join(root, "intent.yaml"), []byte("pending: fixture\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			syncer.store = wrapped
			result, err := syncer.Prune(ctx, PruneOptions{RetentionDays: 7})
			if mode == "changed" {
				if err != nil || result.ChangedOrPreserved != 1 || result.Purged != 0 {
					t.Fatal(result, err)
				}
			} else if err == nil || result.Failed != 1 || !result.OutcomeUnknown {
				t.Fatal(result, err)
			}
			if _, err = os.Stat(filepath.Join(root, "resolved", hash+".yaml")); err != nil {
				t.Fatal("protected source missing", err)
			}
			if mode == "journal" {
				if _, err = os.Stat(filepath.Join(root, "intent.yaml")); err != nil {
					t.Fatal("journal recovered", err)
				}
			}
		})
	}
}

func TestPruneDisabledAndUnknown(t *testing.T) {
	ctx := context.Background()
	store := makeStore(t, privateRoot(t), "prune-unknown")
	add(t, store, 821, clock)
	reader, err := NewStatusReader(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	root, _, err := store.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct{ name, data string }{{stateFile, `{"version":1}`}, {"intent.yaml", "pending: invalid\n"}} {
		if err = os.WriteFile(filepath.Join(root, sample.name), []byte(sample.data), 0600); err != nil {
			t.Fatal(err)
		}
		before := pendingFiles(t, root)
		for _, days := range []int{0, -1} {
			value, err := reader.Prune(ctx, PushBoth, PruneOptions{RetentionDays: days})
			if err != nil || !value.Runtimes.Disabled || !value.SlowSQL.Disabled {
				t.Fatal("disabled read unavailable/bad source", value, err)
			}
		}
		value, err := reader.Prune(ctx, PushRuntimes, PruneOptions{RetentionDays: 1, DryRun: true})
		if err == nil || value.Runtimes.Known {
			t.Fatal("bad state became zero", value, err)
		}
		if !reflect.DeepEqual(before, pendingFiles(t, root)) {
			t.Fatal("failure/disabled mutated source")
		}
		if err = os.Remove(filepath.Join(root, sample.name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPruneSlowAndBusy(t *testing.T) {
	ctx := context.Background()
	store, input, now := slowSyncStore(t)
	hash, _, err := store.ReportSlowQuery(ctx, input, now)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := store.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(root, "open", hash+".yaml"), filepath.Join(root, "resolved", hash+".yaml")); err != nil {
		t.Fatal(err)
	}
	reader, err := NewStatusReader(nil, store)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	snapshots, err := store.ReadOnlySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshots[0]
	if err = reader.slow.save(state{Version: 1, Confirmed: map[string]Confirmed{hash: {Digest: snapshot.Digest, Timestamp: snapshot.Timestamp, Bucket: snapshot.Bucket}}}); err != nil {
		t.Fatal(err)
	}
	<-reader.slow.gate
	value, err := reader.Prune(ctx, PushSlowSQL, PruneOptions{RetentionDays: 1})
	reader.slow.gate <- struct{}{}
	if err != nil || !value.SlowSQL.Busy || value.SlowSQL.Known {
		t.Fatal(value, err)
	}
	value, err = reader.Prune(ctx, PushSlowSQL, PruneOptions{RetentionDays: 1})
	if err != nil || value.SlowSQL.Purged != 1 || !value.SlowSQL.Known {
		t.Fatal(value, err)
	}
}
