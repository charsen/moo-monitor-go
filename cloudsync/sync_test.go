package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/records"
	"go.yaml.in/yaml/v3"
)

type fakeClient func(context.Context, []buffer.Snapshot) (Receipt, error)

func (client fakeClient) Send(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
	return client(ctx, batch)
}

var clock = time.Date(2026, 10, 29, 12, 0, 0, 123000000, time.FixedZone("fixture", 8*3600))

func privateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}
func makeStore(t *testing.T, root, scope string) *buffer.Buffer {
	t.Helper()
	builder, err := records.New(records.Config{MaskKeys: []string{"password", "token"}})
	if err != nil {
		t.Fatal(err)
	}
	cap := 0
	store, err := buffer.New(buffer.Config{Root: root, Scope: scope, DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
func makeSync(t *testing.T, store *buffer.Buffer, client Client) *Syncer {
	t.Helper()
	s, err := New(store, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func event(line int) records.Snapshot {
	return records.Snapshot{Exception: records.Exception{Class: "SyncFixtureError", Code: "0", File: "/fixture/main.go", Line: line, Message: "fixed fixture error"}, Request: records.Request{Method: "CLI"}, Context: records.GoContext{Env: "testing", Project: "fixture", Go: "fixture-go"}, Source: "reportable"}
}
func add(t *testing.T, store *buffer.Buffer, line int, now time.Time) string {
	t.Helper()
	hash, err := store.Record(context.Background(), event(line), now)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
func saved(batch []buffer.Snapshot) Receipt {
	receipt := Receipt{Saved: int64(len(batch))}
	for index, snapshot := range batch {
		receipt.Results = append(receipt.Results, Item{Index: index, Hash: snapshot.Hash, Status: "saved"})
	}
	return receipt
}
func text(value string) *string { return &value }

func TestReceiptSourceFixture(t *testing.T) {
	data, err := os.ReadFile("../docs/contracts/fixtures/cloud-receipts.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name                     string
			Raw                      json.RawMessage
			Saved, Filtered, Skipped int64
			Records                  []struct{ Hash string }
			Output                   json.RawMessage
		}
		Empty []struct {
			Wire   string `json:"wire_results_json"`
			Output json.RawMessage
		} `json:"empty_cases"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			batch := []buffer.Snapshot{}
			for _, record := range tc.Records {
				batch = append(batch, buffer.Snapshot{Hash: record.Hash})
			}
			body, _ := json.Marshal(map[string]any{"ok": true, "saved": tc.Saved, "filtered": tc.Filtered, "skipped": tc.Skipped, "results": tc.Raw})
			receipt, err := DecodeReceipt(200, body, batch)
			if bytes.Equal(tc.Output, []byte("null")) {
				if err == nil {
					t.Fatal("invalid source sample accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			actual, _ := json.Marshal(receipt.Results)
			var got, want any
			json.Unmarshal(actual, &got)
			json.Unmarshal(tc.Output, &want)
			if !reflect.DeepEqual(got, want) {
				t.Fatal("source receipt differs")
			}
		})
	}
	for _, tc := range fixture.Empty {
		body := []byte(`{"ok":true,"saved":0,"filtered":0,"skipped":0,"results":` + tc.Wire + `}`)
		_, err := DecodeReceipt(200, body, nil)
		if (err != nil) != bytes.Equal(tc.Output, []byte("null")) {
			t.Fatal("empty source shape", tc.Wire, err)
		}
	}
}

func TestReceiptWireGuards(t *testing.T) {
	batch := []buffer.Snapshot{{Hash: "aaaaaaaaaaaa"}}
	good := `{"ok":true,"saved":1,"filtered":0,"skipped":0}`
	if receipt, err := DecodeReceipt(200, []byte(good), batch); err != nil || !receipt.Legacy {
		t.Fatal("legacy success", err)
	}
	for _, body := range []string{
		`{"ok":true,"saved":null,"filtered":0,"skipped":0}`,
		`{"ok":true,"saved":1.0,"filtered":0,"skipped":0}`,
		`{"ok":true,"saved":1,"filtered":0}`, `{"ok":true,"saved":1,"filtered":0,"skipped":0,"results":null}`,
		`{"ok":true,"saved":0,"filtered":0,"skipped":1}`, `{"ok":"true","saved":1,"filtered":0,"skipped":0}`,
		`{"ok":true,"saved":-1,"filtered":2,"skipped":0}`, good + ` {}`,
	} {
		if _, err := DecodeReceipt(200, []byte(body), batch); err == nil {
			t.Fatal("bad wire accepted", body)
		}
	}
	if _, err := DecodeReceipt(500, []byte(good), batch); err == nil {
		t.Fatal("HTTP error confirmed")
	}
}

func TestSyncPartialAndRestart(t *testing.T) {
	root := privateRoot(t)
	store := makeStore(t, root, "")
	for line := 1; line <= 4; line++ {
		add(t, store, line, clock.Add(time.Duration(line)*time.Second))
	}
	calls := 0
	client := fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
		calls++
		if calls > 1 {
			if len(batch) != 1 || batch[0].Record.Exception.Line != 3 {
				t.Fatal("failure hole did not isolate retry")
			}
			return saved(batch), nil
		}
		return Receipt{Saved: 1, Filtered: 1, Skipped: 2, Results: []Item{{Index: 3, Hash: batch[3].Hash, Status: "skipped", Reason: text("invalid_record")}, {Index: 2, Hash: batch[2].Hash, Status: "skipped", Retryable: true, Reason: text("upsert_failed")}, {Index: 1, Hash: batch[1].Hash, Status: "filtered", Reason: text("ingest_filter")}, {Index: 0, Hash: batch[0].Hash, Status: "saved"}}}, nil
	})
	syncer := makeSync(t, store, client)
	result, err := syncer.Sync(context.Background())
	if !errors.Is(err, ErrPending) || result.Confirmed != 3 || result.Pending != 1 || len(result.Rejected) != 1 || result.Rejected[0].Reason != "invalid_record" {
		t.Fatal("partial", result, err)
	}
	if result.Cursor != clock.Add(2*time.Second).Format("2006-01-02T15:04:05.000Z07:00") {
		t.Fatal("safe prefix", result.Cursor)
	}
	syncer.Close()
	syncer = makeSync(t, store, client)
	result, err = syncer.Sync(context.Background())
	if err != nil || result.Changed != 1 || result.Pending != 0 {
		t.Fatal("retry/restart", result, err)
	}
	result, err = syncer.Sync(context.Background())
	if err != nil || result.Changed != 0 || calls != 2 {
		t.Fatal("anchors repeated", result, err)
	}
	if count, err := store.Count(context.Background(), "open"); err != nil || count != 3 {
		t.Fatal("open anchors", count, err)
	}
}

func TestSyncSameMillisecondAndBackwards(t *testing.T) {
	store := makeStore(t, privateRoot(t), "")
	hash := add(t, store, 1, clock)
	updates := true
	sentCounts := []int64{}
	client := fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
		sentCounts = append(sentCounts, batch[0].Record.Count)
		if updates {
			updates = false
			if _, err := store.Record(ctx, event(1), clock); err != nil {
				t.Fatal(err)
			}
		}
		return saved(batch), nil
	})
	syncer := makeSync(t, store, client)
	if result, err := syncer.Sync(context.Background()); !errors.Is(err, ErrPending) || result.Confirmed != 0 {
		t.Fatal("old same-ms acknowledged", result, err)
	}
	if result, err := syncer.Sync(context.Background()); err != nil || result.Confirmed != 1 {
		t.Fatal("new digest not scanned", result, err)
	}
	if !reflect.DeepEqual(sentCounts, []int64{1, 2}) {
		t.Fatal("same-ms counts", sentCounts)
	}
	if record, err := store.Get(context.Background(), hash); err != nil || record.Count != 2 {
		t.Fatal("anchor lost", err)
	}
	// Clock rollback is a new unconfirmed fingerprint, even below a previously displayed cursor.
	add(t, store, 1, clock.Add(-time.Hour))
	syncer.client = fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
		return Receipt{}, errors.New("fixed transport failure")
	})
	if result, err := syncer.Sync(context.Background()); err == nil || result.Cursor != "" {
		t.Fatal("rollback retained unsafe cursor", result, err)
	}
	syncer.client = fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
		if batch[0].Record.Count != 3 {
			t.Fatal("rollback count")
		}
		return saved(batch), nil
	})
	if result, err := syncer.Sync(context.Background()); err != nil || result.Confirmed != 1 {
		t.Fatal("rollback missed", result, err)
	}
}

func TestSyncBadBatchAndState(t *testing.T) {
	root := privateRoot(t)
	store := makeStore(t, root, "")
	add(t, store, 1, clock)
	add(t, store, 2, clock)
	syncer := makeSync(t, store, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
		receipt := saved(batch)
		receipt.Results[1].Hash = "wrong"
		return receipt, nil
	}))
	if result, err := syncer.Sync(context.Background()); err == nil || result.Confirmed != 0 {
		t.Fatal("bad whole batch mutated", result, err)
	}
	ledger, err := syncer.load()
	if err != nil || len(ledger.Confirmed) != 0 || ledger.Cursor != "" {
		t.Fatal("bad ack advanced state", err)
	}
	for _, bad := range []string{`{`, `{"version":null,"cursor":"","confirmed":{}}`, `{"version":1,"cursor":null,"confirmed":{}}`, `{"version":1,"cursor":"","confirmed":null}`, `{"version":1,"cursor":"","confirmed":{},"future":1}`, `{"version":1,"version":1,"cursor":"","confirmed":{}}`} {
		if err := syncer.root.WriteFile(stateFile, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := syncer.Sync(context.Background()); err == nil {
			t.Fatal("bad state ignored")
		}
		data, _ := syncer.root.ReadFile(stateFile)
		if string(data) != bad {
			t.Fatal("bad state overwritten")
		}
	}
}

func TestSyncStateFailureAtLeastOnce(t *testing.T) {
	store := makeStore(t, privateRoot(t), "")
	add(t, store, 1, clock)
	calls := 0
	syncer := makeSync(t, store, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) { calls++; return saved(batch), nil }))
	replacements := 0
	injected := errors.New("fixed state failure")
	syncer.fault = func(stage string) error {
		if stage == "replace-state" {
			replacements++
			if replacements == 2 {
				return injected
			}
		}
		return nil
	}
	if _, err := syncer.Sync(context.Background()); !errors.Is(err, injected) {
		t.Fatal("state failure missing", err)
	}
	syncer.Close()
	syncer = makeSync(t, store, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) { calls++; return saved(batch), nil }))
	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("at-least-once not replayed", calls)
	}
	if n, err := store.Count(context.Background(), "open"); err != nil || n != 1 {
		t.Fatal("anchor removed")
	}
	// A true partial file write leaves the previously complete ledger intact.
	original, _ := syncer.root.ReadFile(stateFile)
	syncer.writeTest = func(file *os.File, data []byte) (int, error) { return file.Write(data[:len(data)/2]) }
	if _, err := syncer.Sync(context.Background()); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short state write", err)
	}
	after, _ := syncer.root.ReadFile(stateFile)
	if !bytes.Equal(original, after) {
		t.Fatal("short write replaced good state")
	}
}

func TestSyncLocksAndScope(t *testing.T) {
	root := privateRoot(t)
	a := makeStore(t, root, "alpha")
	b := makeStore(t, root, "beta")
	add(t, a, 1, clock)
	add(t, b, 1, clock)
	entered, release := make(chan struct{}), make(chan struct{})
	slow := makeSync(t, a, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
		close(entered)
		<-release
		return saved(batch), nil
	}))
	done := make(chan error, 1)
	go func() { _, err := slow.Sync(context.Background()); done <- err }()
	<-entered
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	if result, err := slow.Sync(context.Background()); err != nil || !result.Busy {
		t.Fatal("same instance overlap", result, err)
	}
	other := makeSync(t, a, fakeClient(func(context.Context, []buffer.Snapshot) (Receipt, error) {
		t.Fatal("overlap sent")
		return Receipt{}, nil
	}))
	if result, err := other.Sync(context.Background()); err != nil || !result.Busy {
		t.Fatal("same scope overlap", result, err)
	}
	independent := makeSync(t, b, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) { return saved(batch), nil }))
	if result, err := independent.Sync(context.Background()); err != nil || result.Confirmed != 1 {
		t.Fatal("scope contention/leak", result, err)
	}
	// Network ownership does not hold the hot record lock.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := a.Record(ctx, event(2), clock); err != nil {
		t.Fatal("network held buffer lock", err)
	}
}

func TestSyncHTTPFixtureAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0}`)
	}))
	defer server.Close()
	store := makeStore(t, privateRoot(t), "")
	add(t, store, 1, clock)
	syncer := makeSync(t, store, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader([]byte(`{"fixed_fake_only":true}`)))
		if err != nil {
			return Receipt{}, err
		}
		response, err := server.Client().Do(request)
		if err != nil {
			return Receipt{}, err
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			return Receipt{}, err
		}
		return DecodeReceipt(response.StatusCode, data, batch)
	}))
	if result, err := syncer.Sync(context.Background()); err != nil || result.Confirmed != 1 {
		t.Fatal("local HTTP adapter", result, err)
	}
}

func seedResolved(t *testing.T, store *buffer.Buffer, hash string) {
	t.Helper()
	record, err := store.Get(context.Background(), hash)
	if err != nil || record == nil {
		t.Fatal(err)
	}
	record.Status = "resolved"
	at, by, note := "fixture-at", "fixture-actor", "fixture-note"
	record.ResolvedAt = &at
	record.ResolvedBy = &by
	record.ResolvedNote = &note
	data, err := yaml.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := store.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "resolved", hash+".yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, "open", hash+".yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestSyncResolvedRaceAndStateFailure(t *testing.T) {
	for _, kind := range []string{"changed-during-send", "state-failure-after-delete"} {
		t.Run(kind, func(t *testing.T) {
			store := makeStore(t, privateRoot(t), "")
			hash := add(t, store, 1, clock)
			seedResolved(t, store, hash)
			calls := 0
			syncer := makeSync(t, store, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
				calls++
				if kind == "changed-during-send" && calls == 1 {
					if _, err := store.Record(ctx, event(1), clock); err != nil {
						t.Fatal(err)
					}
				}
				return saved(batch), nil
			}))
			if kind == "changed-during-send" {
				if result, err := syncer.Sync(context.Background()); !errors.Is(err, ErrPending) || result.Confirmed != 0 {
					t.Fatal("old resolved ack consumed recurrence", result, err)
				}
				record, err := store.Get(context.Background(), hash)
				if err != nil || record == nil || record.Status != "open" || record.Count != 2 {
					t.Fatal("new version lost", err)
				}
				if _, err = syncer.Sync(context.Background()); err != nil {
					t.Fatal("new open did not resend", err)
				}
			} else {
				replacements := 0
				injected := errors.New("fixed post-delete state failure")
				syncer.fault = func(stage string) error {
					if stage == "replace-state" {
						replacements++
						if replacements == 2 {
							return injected
						}
					}
					return nil
				}
				if _, err := syncer.Sync(context.Background()); !errors.Is(err, injected) {
					t.Fatal(err)
				}
				if record, err := store.Get(context.Background(), hash); err != nil || record != nil {
					t.Fatal("confirmed resolved expected deleted", err)
				}
				add(t, store, 1, clock)
				syncer.Close()
				syncer = makeSync(t, store, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
					if batch[0].Bucket != "open" {
						t.Fatal("new bucket")
					}
					return saved(batch), nil
				}))
				if result, err := syncer.Sync(context.Background()); err != nil || result.Confirmed != 1 {
					t.Fatal("new event skipped after state failure", result, err)
				}
				if record, err := store.Get(context.Background(), hash); err != nil || record == nil {
					t.Fatal("new anchor deleted", err)
				}
			}
		})
	}
}

func TestSyncPrivateStateIO(t *testing.T) {
	for _, kind := range []string{"state-link", "lock-link", "directory-sync"} {
		t.Run(kind, func(t *testing.T) {
			store := makeStore(t, privateRoot(t), "")
			add(t, store, 1, clock)
			syncer := makeSync(t, store, fakeClient(func(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) { return saved(batch), nil }))
			if kind == "directory-sync" {
				injected := errors.New("fixed state directory sync failure")
				syncer.fault = func(stage string) error {
					if stage == "sync-directory" {
						return injected
					}
					return nil
				}
				if _, err := syncer.Sync(context.Background()); !errors.Is(err, injected) {
					t.Fatal("directory sync cause", err)
				}
				syncer.fault = nil
				if _, err := syncer.Sync(context.Background()); err != nil {
					t.Fatal("directory sync retry", err)
				}
				return
			}
			name := stateFile
			if kind == "lock-link" {
				name = lockFile
				if err := syncer.root.Remove(name); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(".buffer.lock", filepath.Join(syncer.path, name)); err != nil {
				t.Fatal(err)
			}
			if _, err := syncer.Sync(context.Background()); err == nil {
				t.Fatal("state symlink followed")
			}
		})
	}
}
