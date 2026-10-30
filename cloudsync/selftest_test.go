package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSelfTestHTTPPipeline(t *testing.T) {
	for _, outcome := range []string{"saved", "filtered", "retryable", "rejected", "legacy", "malformed", "heartbeat-failed"} {
		t.Run(outcome, func(t *testing.T) {
			var mu sync.Mutex
			paths := []string{}
			hashes := map[string]string{}
			stamps := map[string]string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				paths = append(paths, r.URL.Path)
				if r.URL.Path == "/api/v1/heartbeat" {
					if outcome == "heartbeat-failed" {
						io.WriteString(w, `{"ok":false}`)
					} else {
						io.WriteString(w, `{"ok":true}`)
					}
					return
				}
				var body struct {
					Token   string           `json:"token"`
					Records []map[string]any `json:"records"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.Token != fakeToken || len(body.Records) != 1 {
					t.Error("selftest not one typed record")
				}
				record := body.Records[0]
				hash, _ := record["hash"].(string)
				hashes[r.URL.Path] = hash
				stamps[r.URL.Path], _ = record["meta"].(map[string]any)["updated_at"].(string)
				if len(hash) != 12 || record["count"] != float64(1) || record["meta"].(map[string]any)["updated_at"] == nil {
					t.Error("selftest identity/count/timestamp invalid")
				}
				if r.URL.Path == "/api/v1/runtimes/intake" {
					if !strings.Contains(record["exception"].(map[string]any)["class"].(string), "SelfTest") {
						t.Error("unrecognizable runtime sample")
					}
				}
				if r.URL.Path == "/api/v1/slow-queries/intake" {
					sql := record["sql"].(map[string]any)
					if !strings.Contains(sql["raw"].(string), "self-test") || !strings.Contains(sql["last"].(string), "1 as connectivity_check") {
						t.Error("slow sample did not use real builder")
					}
				}
				status := "saved"
				retryable := false
				var reason any = nil
				counts := map[string]int{"saved": 1, "filtered": 0, "skipped": 0}
				if outcome == "legacy" {
					io.WriteString(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0}`)
					return
				}
				if outcome == "malformed" {
					io.WriteString(w, `{"ok":true,"saved":1}`)
					return
				}
				if outcome == "filtered" {
					status = "filtered"
					counts["saved"] = 0
					counts["filtered"] = 1
				}
				if outcome == "retryable" || outcome == "rejected" {
					status = "skipped"
					counts["saved"] = 0
					counts["skipped"] = 1
					retryable = outcome == "retryable"
					reason = "token=" + fakeToken
				}
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": counts["saved"], "filtered": counts["filtered"], "skipped": counts["skipped"], "results": []any{map[string]any{"index": 0, "hash": hash, "status": status, "retryable": retryable, "reason": reason}}})
			}))
			defer server.Close()
			client := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken})
			moment := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
			checker, err := NewSelfTest(client, SelfTestConfig{Meta: healthMeta(), Project: "self-test", GoravelVersion: "1.18.0", Clock: func() time.Time { now := moment; moment = moment.Add(time.Second); return now }})
			if err != nil {
				t.Fatal(err)
			}
			result, err := checker.Run(context.Background(), SelfTestBoth)
			if (err == nil) != (outcome == "saved") {
				t.Fatal("not saved self-test passed", outcome)
			}
			if err != nil && strings.Contains(err.Error(), fakeToken) {
				t.Fatal("receipt reason leaked")
			}
			if outcome == "heartbeat-failed" {
				if len(paths) != 1 || result.Runtime.Attempted || result.SlowSQL.Attempted {
					t.Fatal("heartbeat gate allowed intake")
				}
				return
			}
			if len(paths) != 3 || !result.Runtime.Attempted || !result.SlowSQL.Attempted {
				t.Fatal("one failed type prevented other self-test")
			}
			mu.Lock()
			first := hashes["/api/v1/runtimes/intake"]
			firstStamp := stamps["/api/v1/runtimes/intake"]
			slowStamp := stamps["/api/v1/slow-queries/intake"]
			mu.Unlock()
			if firstStamp == "" || firstStamp != slowStamp {
				t.Fatal("one selftest run used inconsistent timestamp")
			}
			result2, _ := checker.Run(context.Background(), SelfTestRuntimes)
			mu.Lock()
			nextStamp := stamps["/api/v1/runtimes/intake"]
			mu.Unlock()
			if result2.Runtime.Hash != first || result2.SlowSQL.Attempted || nextStamp == firstStamp {
				t.Fatal("sample hash unstable/type isolation failed")
			}
		})
	}
}
func TestSelfTestInputGate(t *testing.T) {
	calls := 0
	client := httpClient(t, HTTPConfig{BaseURL: "http://fake.invalid", Token: fakeToken, Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("fake token " + fakeToken)
	})}})
	checker, err := NewSelfTest(client, SelfTestConfig{Meta: healthMeta()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = checker.Run(context.Background(), SelfTestType("invalid")); !errors.Is(err, ErrSelfTestConfig) || calls != 0 {
		t.Fatal("invalid type made request")
	}
	if _, err = checker.Run(nil, SelfTestBoth); !errors.Is(err, ErrSelfTestConfig) || calls != 0 {
		t.Fatal("nil context made request")
	}
	result, err := checker.Run(context.Background(), SelfTestBoth)
	if err == nil || calls != 1 || result.Heartbeat || strings.Contains(err.Error(), fakeToken) {
		t.Fatal("unsafe transport failure or intake after failure")
	}
}
