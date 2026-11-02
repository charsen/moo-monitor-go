package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type replayRecord struct {
	Hash  string `json:"hash"`
	Count int64  `json:"count"`
}

func readReplay(t *testing.T, r *http.Request) []replayRecord {
	t.Helper()
	var body struct {
		Token   string         `json:"token"`
		Records []replayRecord `json:"records"`
	}
	if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || body.Token != fakeToken || len(body.Records) == 0 {
		t.Error("invalid replay HTTP envelope")
	}
	return body.Records
}
func replayReply(w http.ResponseWriter, records []replayRecord, outcome func(string) string) {
	counts := map[string]int{"saved": 0, "filtered": 0, "skipped": 0}
	items := []Item{}
	for i, record := range records {
		status := "saved"
		if outcome != nil {
			status = outcome(record.Hash)
		}
		item := Item{Index: i, Hash: record.Hash, Status: status}
		if status == "retryable" {
			reason := "upsert_failed"
			item.Status = "skipped"
			item.Retryable = true
			item.Reason = &reason
		}
		counts[item.Status]++
		items = append(items, item)
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": counts["saved"], "filtered": counts["filtered"], "skipped": counts["skipped"], "results": items})
}
func newReplayHTTP(t *testing.T, handle http.HandlerFunc) *HTTPClient {
	t.Helper()
	server := httptest.NewServer(handle)
	t.Cleanup(server.Close)
	return httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken})
}

func TestAllReplaySelectionAndPreview(t *testing.T) {
	store := makeStore(t, privateRoot(t), "all-selection")
	var mu sync.Mutex
	sent := map[string]int{}
	client := newReplayHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		batch := readReplay(t, r)
		mu.Lock()
		for _, record := range batch {
			sent[record.Hash]++
			if record.Count != 1 {
				t.Error("replay changed cumulative count")
			}
		}
		mu.Unlock()
		replayReply(w, batch, nil)
	})
	syncer, err := New(store, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer syncer.Close()
	acknowledged := addDiscardEvent(t, store, 301, "local")
	overlap := addDiscardEvent(t, store, 302, "local")
	if _, err = syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	suppressed := addDiscardEvent(t, store, 303, "local")
	pending := addDiscardEvent(t, store, 304, "local")
	snapshots, err := store.ReadOnlySnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]suppressedVersion{}
	for _, snapshot := range snapshots {
		if snapshot.Hash == suppressed || snapshot.Hash == overlap {
			entries[snapshot.Hash] = suppressSnapshot(snapshot)
		}
	}
	if err = syncer.saveSuppression(suppressionLedger{Version: 1, Entries: entries}); err != nil {
		t.Fatal(err)
	}
	path, _, err := store.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	suppBefore, err := os.ReadFile(filepath.Join(path, runtimeSuppressionFile))
	if err != nil {
		t.Fatal(err)
	}
	stats, err := syncer.Stats(context.Background())
	if err != nil || stats.Counts.ConfirmedOpen != 2 || stats.Counts.SuppressedOpen != 1 || stats.Counts.PendingOpen != 1 {
		t.Fatal("normal classification precedence", stats, err)
	}
	incremental, err := syncer.Preview(context.Background(), PreviewOptions{Mode: Incremental})
	if err != nil || incremental.Selected == nil || *incremental.Selected != 1 {
		t.Fatal("incremental preview", incremental, err)
	}
	all, err := syncer.Preview(context.Background(), PreviewOptions{Mode: All})
	if err != nil || all.Selected == nil || *all.Selected != 2 {
		t.Fatal("All preview inferred from classification instead of exact suppression", all, err)
	}
	result, err := syncer.SyncWithOptions(context.Background(), SyncOptions{Mode: All})
	if err != nil || result.Changed != 2 || result.Confirmed != 2 || result.Remaining != 0 || result.Pending != 0 {
		t.Fatal("all selection/result", result, err)
	}
	mu.Lock()
	a, o, s, p := sent[acknowledged], sent[overlap], sent[suppressed], sent[pending]
	mu.Unlock()
	if a != 2 || o != 1 || s != 0 || p != 1 {
		t.Fatal("All skipped ACK or overrode exact suppression", a, o, s, p)
	}
	after, err := os.ReadFile(filepath.Join(path, runtimeSuppressionFile))
	if err != nil || string(after) != string(suppBefore) {
		t.Fatal("All rewrote suppression")
	}
	if result, err = syncer.Sync(context.Background()); err != nil || result.Sent != 0 {
		t.Fatal("default Sync stopped being incremental", result, err)
	}
}

func TestAllReplayHistoricACKFailure(t *testing.T) {
	for _, mode := range []string{"retryable", "http-failure", "bad-receipt"} {
		t.Run(mode, func(t *testing.T) {
			store := makeStore(t, privateRoot(t), "all-history")
			var failure atomic.Bool
			var calls atomic.Int64
			client := newReplayHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				batch := readReplay(t, r)
				calls.Add(1)
				if failure.Load() {
					switch mode {
					case "http-failure":
						w.WriteHeader(503)
						io.WriteString(w, fakeToken)
						return
					case "bad-receipt":
						io.WriteString(w, `{"ok":true,"saved":1}`)
						return
					case "retryable":
						replayReply(w, batch, func(string) string { return "retryable" })
						return
					}
				}
				replayReply(w, batch, nil)
			})
			syncer, err := New(store, client, Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer syncer.Close()
			hash := addDiscardEvent(t, store, 311, "local")
			if _, err = syncer.Sync(context.Background()); err != nil {
				t.Fatal(err)
			}
			path, _, err := store.ScopeRoot()
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(path, stateFile))
			if err != nil {
				t.Fatal(err)
			}
			failure.Store(true)
			result, err := syncer.SyncWithOptions(context.Background(), SyncOptions{Mode: All})
			if err == nil || result.Pending != 0 || result.Remaining != 1 || result.Sent != 1 || result.Confirmed != 0 {
				t.Fatal("historical ACK made failed replay look complete", result, err)
			}
			if strings.Contains(err.Error(), fakeToken) {
				t.Fatal("replay leaked Cloud error")
			}
			if mode == "retryable" && !errors.Is(err, ErrPending) {
				t.Fatal("Remaining did not fail operation")
			}
			after, err := os.ReadFile(filepath.Join(path, stateFile))
			if err != nil || string(after) != string(before) {
				t.Fatal("failed All cleared/rewrote historical ACK")
			}
			record, err := store.Get(context.Background(), hash)
			if err != nil || record.Count != 1 {
				t.Fatal("failed replay changed source")
			}
			if _, err = syncer.SyncWithOptions(context.Background(), SyncOptions{Mode: "invalid"}); !errors.Is(err, ErrPushConfig) || calls.Load() != 2 {
				t.Fatal("invalid mode sent or mutated")
			}
			failure.Store(false)
			result, err = syncer.SyncWithOptions(context.Background(), SyncOptions{Mode: All})
			if err != nil || result.Remaining != 0 || calls.Load() != 3 {
				t.Fatal("explicit retry did not replay eligible snapshot", result, err)
			}
		})
	}
}

func TestAllReplayPartialNewVersionAndPersistence(t *testing.T) {
	for _, mode := range []string{"partial", "new-version", "state-failure"} {
		t.Run(mode, func(t *testing.T) {
			store := makeStore(t, privateRoot(t), "all-progress")
			var replay atomic.Bool
			var received atomic.Bool
			var oldHash, pendingHash string
			client := newReplayHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				batch := readReplay(t, r)
				if replay.Load() {
					received.Store(true)
					if mode == "new-version" {
						sample := event(321)
						sample.Context.Env = "local"
						ctx, cancel := context.WithTimeout(context.Background(), time.Second)
						defer cancel()
						if _, err := store.Record(ctx, sample, clock); err != nil {
							t.Error("network held buffer lock", err)
						}
					}
					if mode == "partial" {
						replayReply(w, batch, func(hash string) string {
							if hash == oldHash {
								return "retryable"
							}
							return "filtered"
						})
						return
					}
				}
				replayReply(w, batch, nil)
			})
			syncer, err := New(store, client, Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer syncer.Close()
			oldHash = addDiscardEvent(t, store, 321, "local")
			if _, err = syncer.Sync(context.Background()); err != nil {
				t.Fatal(err)
			}
			if mode != "new-version" {
				pendingHash = addDiscardEvent(t, store, 322, "local")
			}
			sentinel := errors.New("fixed All state persistence failure")
			if mode == "state-failure" {
				syncer.fault = func(stage string) error {
					if received.Load() && stage == "replace-state" {
						return sentinel
					}
					return nil
				}
			}
			replay.Store(true)
			result, err := syncer.SyncWithOptions(context.Background(), SyncOptions{Mode: All})
			switch mode {
			case "partial":
				if !errors.Is(err, ErrPending) || result.Remaining != 1 || result.Pending != 0 || result.Confirmed != 1 {
					t.Fatal("partial progress conflated old ACK", result, err)
				}
			case "new-version":
				if err == nil || result.Remaining != 1 || result.Confirmed != 0 {
					t.Fatal("old receipt acknowledged new same-ms version", result, err)
				}
			case "state-failure":
				if !errors.Is(err, sentinel) || result.Remaining != 0 || result.Confirmed != 2 {
					t.Fatal("Remaining0 concealed persistence error", result, err)
				}
			}
			syncer.fault = nil
			stats, err := syncer.Stats(context.Background())
			if err != nil || !stats.Known {
				t.Fatal(err)
			}
			expected := 0
			if mode == "new-version" || mode == "state-failure" {
				expected = 1
			}
			if stats.Counts.PendingOpen != expected {
				t.Fatal("disk truth after All outcome", stats)
			}
			if mode == "new-version" {
				record, err := store.Get(context.Background(), oldHash)
				if err != nil || record.Count != 2 {
					t.Fatal("new occurrence overwritten")
				}
			}
			if pendingHash != "" {
				record, err := store.Get(context.Background(), pendingHash)
				if err != nil || record.Count != 1 {
					t.Fatal("replay mutated pending counter")
				}
			}
		})
	}
}

func TestAllPusherRuntimeSlowAndTick(t *testing.T) {
	runtime := makeStore(t, privateRoot(t), "all-pusher")
	addDiscardEvent(t, runtime, 331, "local")
	slow, input, now := slowSyncStore(t)
	if _, _, err := slow.ReportSlowQuery(context.Background(), input, now); err != nil {
		t.Fatal(err)
	}
	var runtimeCalls, slowCalls, healthCalls atomic.Int64
	var replayFailure atomic.Bool
	client := newReplayHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		batch := readReplay(t, r)
		if r.URL.Path == "/api/v1/runtimes/intake" {
			runtimeCalls.Add(1)
			if replayFailure.Load() {
				replayReply(w, batch, func(string) string { return "retryable" })
				return
			}
		} else if r.URL.Path == "/api/v1/slow-queries/intake" {
			slowCalls.Add(1)
		} else {
			t.Error("wrong replay endpoint")
		}
		replayReply(w, batch, nil)
	})
	pusher, err := NewPusher(runtime, slow, client, PushConfig{Runtimes: true, SlowSQL: true, Timeout: time.Second, AfterPush: func(context.Context) error { healthCalls.Add(1); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer pusher.Close(context.Background())
	if _, err = pusher.Push(context.Background(), PushBoth); err != nil {
		t.Fatal(err)
	}
	if result, err := pusher.Tick(context.Background()); err != nil || result.Runtimes.Sent != 0 || result.SlowSQL.Sent != 0 {
		t.Fatal("automatic Tick replays history", result, err)
	}
	replayFailure.Store(true)
	result, err := pusher.PushWithOptions(context.Background(), PushBoth, SyncOptions{Mode: All})
	if err == nil || !result.Runtimes.Failed || result.Runtimes.Pending != 0 || result.Runtimes.Remaining != 1 || result.SlowSQL.Failed || result.SlowSQL.Remaining != 0 || runtimeCalls.Load() != 2 || slowCalls.Load() != 2 || healthCalls.Load() != 3 {
		t.Fatal("All lost per-type failure/continuation/health", result, err)
	}
	replayFailure.Store(false)
	if result, err = pusher.Tick(context.Background()); err != nil || result.Runtimes.Sent != 0 || result.SlowSQL.Sent != 0 {
		t.Fatal("All changed future Tick selection", result, err)
	}
}
