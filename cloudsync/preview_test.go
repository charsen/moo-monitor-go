package cloudsync

import (
	"context"
	"reflect"
	"testing"

	"gitee.com/charsen/moo-monitor-go/buffer"
)

func TestPreviewReadOnlyAndSelection(t *testing.T) {
	ctx := context.Background()
	store := makeStore(t, privateRoot(t), "preview")
	for _, line := range []int{701, 702, 703} {
		add(t, store, line, clock)
	}
	calls := 0
	syncer := makeSync(t, store, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
		calls++
		if len(batch) != 1 || batch[0].Record.Exception.Line != 703 {
			t.Fatal("preview/send selection differs", batch)
		}
		return saved(batch), nil
	}))
	snapshots, err := store.ReadOnlySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ledger := state{Version: 1, Confirmed: map[string]Confirmed{}}
	suppressed := suppressionLedger{Version: 1, Entries: map[string]suppressedVersion{}}
	for _, s := range snapshots {
		switch s.Record.Exception.Line {
		case 701:
			ledger.Confirmed[s.Hash] = Confirmed{Digest: s.Digest, Timestamp: s.Timestamp, Bucket: s.Bucket}
		case 702:
			suppressed.Entries[s.Hash] = suppressSnapshot(s)
		}
	}
	if err = syncer.save(ledger); err != nil {
		t.Fatal(err)
	}
	if err = syncer.saveSuppression(suppressed); err != nil {
		t.Fatal(err)
	}
	root, _, err := store.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	before := pendingFiles(t, root)
	value, err := syncer.Preview(ctx, PreviewOptions{Mode: Incremental})
	if err != nil || !value.Known || value.WouldSend == nil || *value.WouldSend != 1 || value.Counts.ConfirmedOpen != 1 || value.Counts.SuppressedOpen != 1 {
		t.Fatal(value, err)
	}
	if calls != 0 || !reflect.DeepEqual(before, pendingFiles(t, root)) {
		t.Fatal("preview changed state or sent")
	}
	if _, err = syncer.Preview(ctx, PreviewOptions{Mode: "unsupported"}); err == nil {
		t.Fatal("unsupported mode accepted")
	}
	result, err := syncer.Sync(ctx)
	if err != nil || result.Changed != *value.WouldSend || calls != 1 {
		t.Fatal(result, err)
	}
}
