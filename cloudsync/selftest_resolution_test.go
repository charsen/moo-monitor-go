package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSelfTestResolutionHTTP(t *testing.T) {
	for _, mode := range []string{"both", "runtime", "false", "slow-only", "filtered", "legacy", "retryable", "bad-intake", "forbidden", "bad-resolution", "timeout", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			paths := []string{}
			runtimeHash := ""
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Token, Hash, Note string
					ResolvedBy        string `json:"resolved_by"`
					Records           []struct {
						Hash string `json:"hash"`
					} `json:"records"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.Token != fakeToken {
					t.Error("fake auth/body")
					w.WriteHeader(400)
					return
				}
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				switch r.URL.Path {
				case "/api/v1/heartbeat":
					fmt.Fprint(w, `{"ok":true}`)
				case "/api/v1/runtimes/resolve":
					mu.Lock()
					hash := runtimeHash
					mu.Unlock()
					if body.Hash != hash || body.Hash == "" || body.Note != "moo:cloud:test 自检记录" || body.ResolvedBy != "moo:cloud:test" {
						t.Error("resolved other than this fixed selftest")
					}
					switch mode {
					case "forbidden":
						w.WriteHeader(403)
						fmt.Fprintf(w, `{"error":%q}`, fakeToken)
					case "bad-resolution":
						fmt.Fprint(w, `{"ok":true,"runtime":{"hash":"fedcba987654","status":"resolved"}}`)
					case "timeout":
						<-r.Context().Done()
					case "canceled":
						cancel()
						<-r.Context().Done()
					default:
						json.NewEncoder(w).Encode(map[string]any{"ok": true, "runtime": map[string]any{"hash": body.Hash, "status": "resolved"}, "already_resolved": false})
					}
				default:
					if len(body.Records) != 1 {
						t.Error("not one selftest record")
						w.WriteHeader(400)
						return
					}
					hash := body.Records[0].Hash
					status := "saved"
					saved, filtered, skipped := 1, 0, 0
					retryable := false
					if r.URL.Path == "/api/v1/runtimes/intake" {
						mu.Lock()
						runtimeHash = hash
						mu.Unlock()
						switch mode {
						case "legacy":
							fmt.Fprint(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0}`)
							return
						case "bad-intake":
							fmt.Fprint(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0,"results":[]}`)
							return
						case "filtered":
							status = "filtered"
							saved = 0
							filtered = 1
						case "retryable":
							status = "skipped"
							saved = 0
							skipped = 1
							retryable = true
						}
					} else if r.URL.Path != "/api/v1/slow-queries/intake" {
						t.Error("unexpected path")
						w.WriteHeader(404)
						return
					}
					reason := any(nil)
					if retryable {
						reason = "fixed rejection"
					}
					json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": saved, "filtered": filtered, "skipped": skipped, "results": []any{map[string]any{"index": 0, "hash": hash, "status": status, "retryable": retryable, "reason": reason}}})
				}
			}))
			defer server.Close()
			timeout := 2 * time.Second
			if mode == "timeout" {
				timeout = time.Second
			}
			client := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken, Timeout: timeout})
			checker, err := NewSelfTest(client, SelfTestConfig{Meta: healthMeta(), Project: "resolution-fixture", GoravelVersion: "fixture", RuntimeResolver: client})
			if err != nil {
				t.Fatal(err)
			}
			kind := SelfTestBoth
			if mode == "runtime" {
				kind = SelfTestRuntimes
			}
			if mode == "slow-only" {
				kind = SelfTestSlowSQL
			}
			var result SelfTestResult
			if mode == "false" {
				result, err = checker.Run(ctx, kind)
			} else {
				result, err = checker.RunWithOptions(ctx, kind, SelfTestOptions{ResolveRuntime: true})
			}
			primaryFailure := mode == "filtered" || mode == "legacy" || mode == "retryable" || mode == "bad-intake" || mode == "canceled"
			if (err != nil) != primaryFailure {
				t.Fatal("resolution changed saved result", err, result.Resolution)
			}
			if mode == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal("parent cancellation lost", err)
			}
			mu.Lock()
			actual := append([]string(nil), paths...)
			mu.Unlock()
			expected := []string{"/api/v1/heartbeat"}
			if kind != SelfTestSlowSQL {
				expected = append(expected, "/api/v1/runtimes/intake")
			}
			shouldResolve := mode != "false" && mode != "slow-only" && mode != "filtered" && mode != "legacy" && mode != "retryable" && mode != "bad-intake"
			if shouldResolve {
				expected = append(expected, "/api/v1/runtimes/resolve")
				if mode == "timeout" {
					expected = append(expected, "/api/v1/runtimes/resolve")
				}
			}
			if kind != SelfTestRuntimes && mode != "canceled" {
				expected = append(expected, "/api/v1/slow-queries/intake")
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatal("selftest request order", actual, expected)
			}
			if shouldResolve && (!result.Runtime.Saved || !result.Resolution.Attempted || result.Resolution.Hash != result.Runtime.Hash) {
				t.Fatal("saved/resolve scope", result.Resolution)
			}
			outcome := "resolved"
			switch mode {
			case "false":
				outcome = "not_requested"
			case "slow-only":
				outcome = "not_applicable"
			case "filtered", "legacy", "retryable", "bad-intake":
				outcome = "runtime_not_saved"
			case "forbidden":
				outcome = "permission_denied"
			case "bad-resolution":
				outcome = "invalid_receipt"
			case "timeout":
				outcome = "timeout"
			case "canceled":
				outcome = "canceled"
			}
			if result.Resolution.Outcome != outcome || result.Resolution.Resolved != (outcome == "resolved") {
				t.Fatal("resolution outcome", result.Resolution)
			}
			if strings.Contains(fmt.Sprint(result.Resolution), fakeToken) {
				t.Fatal("free Cloud error escaped")
			}
		})
	}
}

type panicRuntimeResolver struct{}

func (panicRuntimeResolver) ResolveRuntime(context.Context, string, *string, *string) (MCPObject, error) {
	panic("fixed-private-resolution-data")
}
func TestSelfTestResolutionBoundary(t *testing.T) {
	result := resolveSelfTestRuntime(context.Background(), nil, "abcdef012345")
	if result.Attempted || result.Outcome != "unavailable" {
		t.Fatal(result)
	}
	result = resolveSelfTestRuntime(context.Background(), panicRuntimeResolver{}, "abcdef012345")
	if !result.Attempted || result.Resolved || result.Outcome != "resolver_panic" || strings.Contains(fmt.Sprint(result), "private") {
		t.Fatal(result)
	}
}
