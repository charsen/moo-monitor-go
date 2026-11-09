package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/slowquery"
)

func slowSyncStore(t *testing.T) (*buffer.SlowBuffer, slowquery.Input, time.Time) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	builder, err := slowquery.New(slowquery.Config{Enabled: true, MaskKeys: []string{"token"}, Env: "testing", Project: "slow-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	cap := 0
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "fixture", DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store, slowquery.Input{Raw: "select * from foods where token=? and id=?", Bindings: []any{"fake-secret", int64(42)}, TookMillis: 123.455, File: "app/query.go", Line: 11}, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
}

type slowSender func(context.Context, []buffer.SlowSnapshot) (Receipt, error)

func (f slowSender) SendSlow(ctx context.Context, batch []buffer.SlowSnapshot) (Receipt, error) {
	return f(ctx, batch)
}
func TestSlowSyncVersionsAndReceipts(t *testing.T) {
	store, input, now := slowSyncStore(t)
	hashes := []string{}
	for i := 0; i < 4; i++ {
		sample := input
		sample.Line += i
		hash, _, err := store.ReportSlowQuery(context.Background(), sample, now.Add(time.Duration(i)*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, hash)
	}
	first := true
	client := slowSender(func(ctx context.Context, batch []buffer.SlowSnapshot) (Receipt, error) {
		if !first {
			return Receipt{Saved: int64(len(batch)), Legacy: true}, nil
		}
		first = false
		// The same millisecond version changes while transport is in progress.
		if _, _, err := store.ReportSlowQuery(ctx, input, now); err != nil {
			t.Fatal(err)
		}
		temporary, permanent := "upsert_failed", "invalid_record"
		receipt := Receipt{Saved: 1, Filtered: 1, Skipped: 2}
		for i, snapshot := range batch {
			item := Item{Index: i, Hash: snapshot.Hash}
			switch snapshot.Hash {
			case hashes[0]:
				item.Status = "saved"
			case hashes[1]:
				item.Status = "filtered"
			case hashes[2]:
				item.Status = "skipped"
				item.Retryable = true
				item.Reason = &temporary
			case hashes[3]:
				item.Status = "skipped"
				item.Reason = &permanent
			}
			receipt.Results = append(receipt.Results, item)
		}
		return receipt, nil
	})
	syncer, err := NewSlow(store, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := syncer.Sync(context.Background())
	if !errors.Is(err, ErrPending) || result.Sent != 4 || result.Confirmed != 2 || result.Pending != 2 || len(result.Rejected) != 1 {
		t.Fatal("SQL partial version decision", err, result)
	}
	record, _ := store.Get(context.Background(), hashes[0])
	if record == nil || record.Count != 2 {
		t.Fatal("old ack lost same-ms update")
	}
	syncer.Close()
	syncer, err = NewSlow(store, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer syncer.Close()
	result, err = syncer.Sync(context.Background())
	if err != nil || result.Sent != 2 || result.Pending != 0 {
		t.Fatal("failed hole/restart", err, result)
	}
	result, err = syncer.Sync(context.Background())
	if err != nil || result.Sent != 0 {
		t.Fatal("confirmed open anchors resent", err)
	}
	if count, err := store.Count(context.Background(), "open"); err != nil || count != 3 {
		t.Fatal("open anchor missing", err)
	}
	root, _, _ := store.ScopeRoot()
	files, err := filepath.Glob(filepath.Join(root, "cloud-rejected", "*.receipt.json"))
	if err != nil || len(files) != 1 {
		t.Fatal("rejection not persistent")
	}
}
func TestSlowSyncBadReceiptAndStateFailure(t *testing.T) {
	store, input, now := slowSyncStore(t)
	hash, _, err := store.ReportSlowQuery(context.Background(), input, now)
	if err != nil {
		t.Fatal(err)
	}
	bad := true
	client := slowSender(func(context.Context, []buffer.SlowSnapshot) (Receipt, error) {
		if bad {
			return Receipt{Saved: 2, Legacy: true}, nil
		}
		return Receipt{Saved: 1, Legacy: true}, nil
	})
	syncer, err := NewSlow(store, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer syncer.Close()
	if _, err = syncer.Sync(context.Background()); err == nil {
		t.Fatal("bad receipt accepted")
	}
	state, err := syncer.load()
	if err != nil || len(state.Confirmed) != 0 || state.Cursor != "" {
		t.Fatal("bad receipt advanced SQL state")
	}
	bad = false
	injected := errors.New("fixed SQL ledger failure")
	replacements := 0
	syncer.fault = func(stage string) error {
		if stage == "replace-state" {
			replacements++
			if replacements == 2 {
				return injected
			}
		}
		return nil
	}
	// This is the same shared atomic state writer; verify its visible failure remains attributable.
	if _, err = syncer.Sync(context.Background()); !errors.Is(err, injected) {
		t.Fatal("SQL state fault swallowed or cause lost")
	}
	syncer.fault = nil
	if record, _ := store.Get(context.Background(), hash); record == nil {
		t.Fatal("state fault removed open anchor")
	}
	if _, err = syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestSlowHTTPWireAndSync(t *testing.T) {
	store, input, now := slowSyncStore(t)
	hash, _, err := store.ReportSlowQuery(context.Background(), input, now)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
			t.Error("SQL transport headers")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var envelope map[string]json.RawMessage
		if json.Unmarshal(body, &envelope) != nil {
			t.Error("bad SQL JSON")
			w.WriteHeader(400)
			return
		}
		var token string
		json.Unmarshal(envelope["token"], &token)
		if token != "fixed-fake-token" {
			t.Error("SQL body auth")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/"+slowDiscardPath {
			if string(envelope["env"]) != `"local"` || len(envelope) != 2 {
				t.Error("SQL discard shape")
			}
			io.WriteString(w, `{"ok":true,"deleted":2}`)
			return
		}
		if r.URL.Path != "/"+slowIntakePath || len(envelope) != 2 {
			t.Error("SQL fixed endpoint")
		}
		var batch []map[string]json.RawMessage
		json.Unmarshal(envelope["records"], &batch)
		if len(batch) != 1 {
			t.Error("SQL batch size")
		}
		record := batch[0]
		for _, key := range []string{"exception", "trace", "payload", "source_snippet", "digest", "bucket"} {
			if _, exists := record[key]; exists {
				t.Error("runtime/wrapper field in SQL wire")
			}
		}
		if len(record) != 15 {
			t.Error("SQL whitelist count", len(record))
		}
		if strings.Contains(string(body), "fake-secret") {
			t.Error("SQL secret leaked")
		}
		for _, key := range []string{"sql", "took", "at", "context", "request"} {
			if _, exists := record[key]; !exists {
				t.Error("SQL wire field absent")
			}
		}
		fmt.Fprintf(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0,"results":[{"index":0,"hash":%q,"status":"saved","retryable":false,"reason":null}]}`, hash)
	}))
	defer server.Close()
	client, err := NewHTTPClient(HTTPConfig{BaseURL: server.URL, Token: "fixed-fake-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	syncer, err := NewSlow(store, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer syncer.Close()
	result, err := syncer.Sync(context.Background())
	if err != nil || result.Confirmed != 1 || result.Pending != 0 {
		t.Fatal("SQL actual HTTP+sync", err)
	}
	if deleted, err := client.DiscardSlowLocalNoise(context.Background()); err != nil || deleted != 2 {
		t.Fatal("SQL discard", err)
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 {
		t.Fatal(err)
	}
	snapshots[0].Record.SQL.Last = string([]byte{255})
	if _, err = client.SendSlow(context.Background(), snapshots); !errors.Is(err, ErrWire) || calls != 2 {
		t.Fatal("bad SQL UTF8 sent request", err)
	}
}
