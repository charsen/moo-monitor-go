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

	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/slowquery"
)

func addDiscardEvent(t *testing.T, store *buffer.Buffer, line int, env string) string {
	t.Helper()
	sample := event(line)
	sample.Context.Env = env
	hash, err := store.Record(context.Background(), sample, clock)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
func discardHTTP(t *testing.T, handle func(http.ResponseWriter, *http.Request) bool) *HTTPClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handle(w, r) {
			return
		}
		var body struct {
			Token   string `json:"token"`
			Records []struct {
				Hash string `json:"hash"`
			} `json:"records"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Token != fakeToken {
			t.Error("invalid intake test envelope")
			w.WriteHeader(400)
			return
		}
		items := []Item{}
		for i, record := range body.Records {
			items = append(items, Item{Index: i, Hash: record.Hash, Status: "saved"})
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": len(items), "filtered": 0, "skipped": 0, "results": items})
	}))
	t.Cleanup(server.Close)
	return httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken})
}
func assertDiscardBody(t *testing.T, r *http.Request) {
	t.Helper()
	var body map[string]any
	if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 2 || body["token"] != fakeToken || body["env"] != "local" {
		t.Error("discard protocol not exact local body")
	}
}
func TestDiscardRuntimeHTTPIntegration(t *testing.T) {
	store := makeStore(t, privateRoot(t), "discard-runtime")
	var discarded atomic.Int64
	var mu sync.Mutex
	sent := map[string]int{}
	client := discardHTTP(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v1/runtimes/discard-local" {
			assertDiscardBody(t, r)
			discarded.Add(1)
			io.WriteString(w, `{"ok":true,"deleted":0}`)
			return true
		}
		var body struct {
			Token   string `json:"token"`
			Records []struct {
				Hash string `json:"hash"`
			} `json:"records"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Token != fakeToken {
			t.Error("invalid real intake")
		}
		items := []Item{}
		mu.Lock()
		for i, record := range body.Records {
			sent[record.Hash]++
			items = append(items, Item{Index: i, Hash: record.Hash, Status: "saved"})
		}
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": len(items), "filtered": 0, "skipped": 0, "results": items})
		return true
	})
	syncer, err := New(store, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer syncer.Close()
	confirmed := addDiscardEvent(t, store, 201, "local")
	if _, err = syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := store.ReadOnlySnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = syncer.saveSuppression(suppressionLedger{Version: 1, Entries: map[string]suppressedVersion{confirmed: suppressSnapshot(before[0])}}); err != nil {
		t.Fatal(err)
	}
	pending := addDiscardEvent(t, store, 202, "local")
	nonlocal := addDiscardEvent(t, store, 203, "production")
	resolved := addDiscardEvent(t, store, 204, "local")
	seedResolved(t, store, resolved)
	deleted := addDiscardEvent(t, store, 205, "local")
	root, _, err := store.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(root, "open", deleted+".yaml"), filepath.Join(root, "deleted", deleted+".yaml")); err != nil {
		t.Fatal(err)
	}
	overflow := filepath.Join(root, "overflow", pending+".yaml")
	if err = os.WriteFile(overflow, []byte("9\n"), 0600); err != nil {
		t.Fatal(err)
	}
	preserved := map[string][]byte{}
	for _, name := range []string{"open/" + confirmed + ".yaml", "open/" + pending + ".yaml", "open/" + nonlocal + ".yaml", "resolved/" + resolved + ".yaml", "deleted/" + deleted + ".yaml", "overflow/" + pending + ".yaml", stateFile} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		preserved[name] = data
	}
	result, err := syncer.DiscardLocalNoise(context.Background())
	if err != nil || !result.CloudConfirmed || result.CloudDeleted != 0 || result.LocalSuppressed != 1 || result.Candidates != 1 || discarded.Load() != 1 {
		t.Fatal("discard confirmation/suppression result", result, err)
	}
	for name, before := range preserved {
		after, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(after) != string(before) {
			t.Fatal("discard changed record/count/overflow/ACK", name)
		}
	}
	stats, err := syncer.Stats(context.Background())
	if err != nil || !stats.Known || stats.Counts.SuppressedOpen != 1 || stats.Counts.ConfirmedOpen != 1 || stats.Counts.PendingOpen != 1 || stats.Counts.PendingResolved != 1 {
		t.Fatal("suppression statistics", stats, err)
	}
	if pushed, err := syncer.Sync(context.Background()); err != nil || pushed.Pending != 0 || pushed.Confirmed != 2 {
		t.Fatal("suppressed snapshot remained pending", pushed, err)
	}
	mu.Lock()
	pendingSends, confirmedSends := sent[pending], sent[confirmed]
	mu.Unlock()
	if pendingSends != 0 || confirmedSends != 1 {
		t.Fatal("suppressed/confirmed anchors sent again")
	}
	record, err := store.Get(context.Background(), pending)
	if err != nil || record.Count != 1 {
		t.Fatal("suppressed anchor lost")
	}
	sample := event(202)
	sample.Context.Env = "local"
	if _, err = store.Record(context.Background(), sample, clock); err != nil {
		t.Fatal(err)
	}
	record, err = store.Get(context.Background(), pending)
	if err != nil || record.Count != 11 {
		t.Fatal("new occurrence reset cumulative/overflow", err)
	}
	stats, err = syncer.Stats(context.Background())
	if err != nil || stats.Counts.SuppressedOpen != 0 || stats.Counts.PendingOpen != 1 {
		t.Fatal("same-ms new version hidden", stats, err)
	}
	if _, err = syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	pendingSends = sent[pending]
	mu.Unlock()
	if pendingSends != 1 {
		t.Fatal("new version never resent")
	}
}
func TestDiscardRaceFailureAndPersistence(t *testing.T) {
	for _, mode := range []string{"cloud-failed", "bad-receipt", "changed", "replace-failed", "sync-failed"} {
		t.Run(mode, func(t *testing.T) {
			store := makeStore(t, privateRoot(t), "discard-race")
			hash := addDiscardEvent(t, store, 211, "local")
			client := discardHTTP(t, func(w http.ResponseWriter, r *http.Request) bool {
				if !strings.HasSuffix(r.URL.Path, "discard-local") {
					return false
				}
				assertDiscardBody(t, r)
				if mode == "cloud-failed" {
					w.WriteHeader(500)
					io.WriteString(w, fakeToken)
					return true
				}
				if mode == "bad-receipt" {
					io.WriteString(w, `{"ok":true,"deleted":null}`)
					return true
				}
				if mode == "changed" {
					sample := event(211)
					sample.Context.Env = "local"
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					if _, err := store.Record(ctx, sample, clock); err != nil {
						t.Error("buffer hot lock held during network", err)
					}
				}
				if mode == "replace-failed" {
					io.WriteString(w, `{"ok":true,"deleted":0}`)
				} else {
					io.WriteString(w, `{"ok":true,"deleted":3}`)
				}
				return true
			})
			syncer, err := New(store, client, Config{})
			if err != nil {
				t.Fatal(err)
			}
			sentinel := errors.New("fixed suppression IO failure")
			if mode == "replace-failed" {
				syncer.fault = func(stage string) error {
					if stage == "replace-state" {
						return sentinel
					}
					return nil
				}
			}
			if mode == "sync-failed" {
				syncer.fault = func(stage string) error {
					if stage == "sync-directory" {
						return sentinel
					}
					return nil
				}
			}
			result, err := syncer.DiscardLocalNoise(context.Background())
			if err != nil && strings.Contains(err.Error(), fakeToken) {
				t.Fatal("Cloud error body leaked")
			}
			switch mode {
			case "cloud-failed", "bad-receipt":
				if err == nil || result.CloudConfirmed || result.LocalSuppressed != 0 {
					t.Fatal("bad remote allowed local suppression", result, err)
				}
			case "changed":
				if err != nil || !result.CloudConfirmed || result.LocalSuppressed != 0 || result.ChangedOrPreserved != 1 {
					t.Fatal("new version suppressed", result, err)
				}
			default:
				if !errors.Is(err, ErrDiscardState) || !errors.Is(err, sentinel) || !result.CloudConfirmed || !result.LocalStateUnknown || result.LocalSuppressed != 0 {
					t.Fatal("partial remote/local outcome hidden", result, err)
				}
			}
			syncer.fault = nil
			if err = syncer.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := New(store, client, Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			stats, err := reopened.Stats(context.Background())
			if err != nil || !stats.Known {
				t.Fatal("reopen state unreadable", err)
			}
			expected := 0
			if mode == "sync-failed" {
				expected = 1
			}
			if stats.Counts.SuppressedOpen != expected {
				t.Fatal("reopen did not reflect actual persisted image", stats)
			}
			record, err := store.Get(context.Background(), hash)
			if err != nil || record == nil {
				t.Fatal("discard failure removed source")
			}
		})
	}
}

func TestDiscardScopeTypeBusyAndCorruption(t *testing.T) {
	root := privateRoot(t)
	first := makeStore(t, root, "scope-one")
	second := makeStore(t, root, "scope-two")
	firstHash := addDiscardEvent(t, first, 231, "local")
	secondHash := addDiscardEvent(t, second, 231, "local")
	if firstHash != secondHash {
		t.Fatal("scope fixture should share hash")
	}
	var runtimeCalls, slowCalls atomic.Int64
	client := discardHTTP(t, func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.HasSuffix(r.URL.Path, "discard-local") {
			return false
		}
		assertDiscardBody(t, r)
		if r.URL.Path == "/api/v1/runtimes/discard-local" {
			runtimeCalls.Add(1)
		} else if r.URL.Path == "/api/v1/slow-queries/discard-local" {
			slowCalls.Add(1)
		} else {
			t.Error("wrong discard endpoint")
		}
		io.WriteString(w, `{"ok":true,"deleted":1}`)
		return true
	})
	firstSync, err := New(first, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer firstSync.Close()
	secondSync, err := New(second, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer secondSync.Close()
	competing, err := New(first, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer competing.Close()
	held, ok, err := firstSync.lock()
	if err != nil || !ok {
		t.Fatal("fixture could not hold type lock")
	}
	defer held.Close()
	result, err := competing.DiscardLocalNoise(context.Background())
	if err != nil || !result.Busy || runtimeCalls.Load() != 0 {
		t.Fatal("busy discard made request")
	}
	builder, err := slowquery.New(slowquery.Config{Enabled: true, Env: "local", Project: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	unlimited := 0
	slow, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "scope-one", DailyCap: &unlimited}, builder)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	if _, _, err = slow.ReportSlowQuery(context.Background(), slowquery.Input{Raw: "select 1", TookMillis: 120, File: "fixture.go"}, clock); err != nil {
		t.Fatal(err)
	}
	slowSync, err := NewSlow(slow, client, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer slowSync.Close()
	result, err = slowSync.DiscardLocalNoise(context.Background())
	if err != nil || !result.CloudConfirmed || result.LocalSuppressed != 1 || slowCalls.Load() != 1 {
		t.Fatal("runtime lock blocked independent SQL type", result, err)
	}
	held.Close()
	result, err = firstSync.DiscardLocalNoise(context.Background())
	if err != nil || result.LocalSuppressed != 1 {
		t.Fatal("scope-one suppression failed", result, err)
	}
	result, err = firstSync.DiscardLocalNoise(context.Background())
	if err != nil || result.LocalSuppressed != 0 || !result.CloudConfirmed || runtimeCalls.Load() != 2 {
		t.Fatal("repeat discard not idempotent locally", result, err)
	}
	stats, err := secondSync.Stats(context.Background())
	if err != nil || stats.Counts.PendingOpen != 1 || stats.Counts.SuppressedOpen != 0 {
		t.Fatal("scope-one suppression crossed scope")
	}
	stats, err = slowSync.Stats(context.Background())
	if err != nil || stats.Counts.SuppressedOpen != 1 {
		t.Fatal("typed suppression lost")
	}
	secondPath, _, err := second.ScopeRoot()
	if err != nil {
		t.Fatal(err)
	}
	corrupt := []byte(`{"version":1,"entries":null}`)
	file := filepath.Join(secondPath, runtimeSuppressionFile)
	if err = os.WriteFile(file, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	calls := runtimeCalls.Load()
	result, err = secondSync.DiscardLocalNoise(context.Background())
	if err == nil || result.CloudConfirmed || runtimeCalls.Load() != calls {
		t.Fatal("bad ledger allowed remote/local action")
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(corrupt) {
		t.Fatal("bad ledger overwritten")
	}
	stats, err = secondSync.Stats(context.Background())
	if err == nil || stats.Known || stats.Counts != nil {
		t.Fatal("bad suppression showed known-zero stats")
	}
	if _, err = second.Get(context.Background(), secondHash); err != nil {
		t.Fatal("corruption touched record")
	}
}
