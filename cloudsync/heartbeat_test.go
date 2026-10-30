package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func healthMeta() HeartbeatMeta {
	return HeartbeatMeta{SDKVersion: "dev", AppEnv: "testing", AppName: "health-fixture", CloudEnabled: true, RuntimeEnabled: true, PushRuntimes: true, PushSlowSQL: true, Schedule: true}
}
func TestHeartbeatWireAndReceipt(t *testing.T) {
	for _, body := range []string{`{"ok":true}`, `{"ok":false}`, `{"ok":"true"}`, `{}`, `{"ok":true,"ok":false}`, `null`, `{"ok":true} {}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/api/v1/heartbeat" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("heartbeat request protocol changed")
				}
				var wire struct {
					Token string         `json:"token"`
					Meta  map[string]any `json:"meta"`
				}
				if json.NewDecoder(r.Body).Decode(&wire) != nil || wire.Token != fakeToken || wire.Meta["sdk"] != "moo-go-monitor" || len(wire.Meta) != 10 {
					t.Error("heartbeat whitelist/token invalid")
				}
				for _, key := range []string{"go_version", "goravel_version", "php_version", "laravel_version"} {
					if _, ok := wire.Meta[key]; ok {
						t.Error("unsupported or fake platform field sent")
					}
				}
				io.WriteString(w, body)
			}))
			defer server.Close()
			client := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken})
			err := client.Heartbeat(context.Background(), healthMeta())
			if (err == nil) != (body == `{"ok":true}`) {
				t.Fatal("heartbeat acknowledgement not strict")
			}
		})
	}
}

type healthTimeout struct{}

func (healthTimeout) Error() string   { return "fake transport timeout" }
func (healthTimeout) Timeout() bool   { return true }
func (healthTimeout) Temporary() bool { return true }
func TestHeartbeatRetriesTimeoutAndSafety(t *testing.T) {
	var calls atomic.Int64
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		remaining := time.Until(mustDeadline(t, r.Context()))
		if remaining > time.Second || remaining < 900*time.Millisecond {
			t.Error("heartbeat timeout was not clamped to one second")
		}
		if calls.Add(1) == 1 {
			return nil, healthTimeout{}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})
	client := httpClient(t, HTTPConfig{BaseURL: "http://fake.invalid", Token: fakeToken, Timeout: 30 * time.Millisecond, Client: &http.Client{Transport: transport}})
	if err := client.Heartbeat(context.Background(), healthMeta()); err != nil || calls.Load() != 2 || client.client.Timeout != 30*time.Millisecond {
		t.Fatal("retry/clamp mutated shared client", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Heartbeat(ctx, healthMeta()); !errors.Is(err, context.Canceled) || calls.Load() != 2 {
		t.Fatal("caller cancellation retried")
	}
	bad := healthMeta()
	bad.AppName = string([]byte{0xff})
	if err := client.Heartbeat(context.Background(), bad); !errors.Is(err, ErrWire) || calls.Load() != 2 {
		t.Fatal("bad UTF8 made request")
	}
	client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("token=" + fakeToken)
	})
	if err := client.Heartbeat(context.Background(), healthMeta()); err == nil || strings.Contains(err.Error(), fakeToken) || !strings.HasPrefix(err.Error(), "heartbeat:") {
		t.Fatal("unsafe heartbeat error")
	}
}
func mustDeadline(t *testing.T, ctx context.Context) time.Time {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("missing attempt timeout")
	}
	return deadline
}

func TestHeartbeatTransportPolicy(t *testing.T) {
	for _, status := range []int{307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var targetCalls atomic.Int64
			target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls.Add(1) }))
			defer target.Close()
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, status) }))
			defer source.Close()
			client := httpClient(t, HTTPConfig{BaseURL: source.URL, Token: fakeToken})
			if err := client.Heartbeat(context.Background(), healthMeta()); !errors.Is(err, ErrHTTPStatus) || targetCalls.Load() != 0 {
				t.Fatal("body token followed redirect")
			}
		})
	}
	var calls atomic.Int64
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
		io.WriteString(w, "token="+fakeToken)
	}))
	defer source.Close()
	client := httpClient(t, HTTPConfig{BaseURL: source.URL, Token: fakeToken})
	if err := client.Heartbeat(context.Background(), healthMeta()); !errors.Is(err, ErrHTTPStatus) || calls.Load() != 1 || strings.Contains(err.Error(), fakeToken) {
		t.Fatal("HTTP failure retried or leaked")
	}
	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"ok":true,"extra":"too large"}`) }))
	defer large.Close()
	client = httpClient(t, HTTPConfig{BaseURL: large.URL, Token: fakeToken, MaxResponseBytes: 8})
	if err := client.Heartbeat(context.Background(), healthMeta()); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatal("heartbeat response unbounded")
	}
	client = httpClient(t, HTTPConfig{BaseURL: "http://fake.invalid", Token: fakeToken, Timeout: 10 * time.Second, Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		remaining := time.Until(mustDeadline(t, r.Context()))
		if remaining > 4*time.Second || remaining < 3900*time.Millisecond {
			t.Error("missing maximum heartbeat timeout")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})}})
	if err := client.Heartbeat(context.Background(), healthMeta()); err != nil || client.client.Timeout != 10*time.Second {
		t.Fatal("maximum timeout changed owner")
	}
}

func TestHeartbeatAfterPush(t *testing.T) {
	for _, mode := range []string{"empty", "confirmed-health-failed", "intake-deadline"} {
		t.Run(mode, func(t *testing.T) {
			var health, intakes atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/heartbeat" {
					health.Add(1)
					if mode == "confirmed-health-failed" {
						w.WriteHeader(503)
					} else {
						io.WriteString(w, `{"ok":true}`)
					}
					return
				}
				intakes.Add(1)
				if mode == "intake-deadline" {
					io.Copy(io.Discard, r.Body)
					select {
					case <-r.Context().Done():
					case <-time.After(time.Second):
					}
					return
				}
				var body struct {
					Records []struct {
						Hash string `json:"hash"`
					} `json:"records"`
				}
				json.NewDecoder(r.Body).Decode(&body)
				items := []Item{}
				for i, record := range body.Records {
					items = append(items, Item{Index: i, Hash: record.Hash, Status: "saved"})
				}
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": len(items), "filtered": 0, "skipped": 0, "results": items})
			}))
			defer server.Close()
			client := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken})
			store := makeStore(t, privateRoot(t), "heartbeat-push")
			if mode != "empty" {
				add(t, store, 91, clock)
			}
			pusher, err := NewPusher(store, nil, client, PushConfig{Runtimes: true, Timeout: 30 * time.Millisecond, AfterPush: func(ctx context.Context) error { return client.Heartbeat(ctx, healthMeta()) }})
			if err != nil {
				t.Fatal(err)
			}
			defer pusher.Close(context.Background())
			result, err := pusher.Tick(context.Background())
			if health.Load() != 1 {
				t.Fatal("completed/failed/empty round did not heartbeat")
			}
			if mode == "empty" {
				if err != nil || result.HealthError != nil || intakes.Load() != 0 {
					t.Fatal("empty heartbeat depended on intake")
				}
			}
			if mode == "confirmed-health-failed" {
				if err != nil || result.Runtimes.Confirmed != 1 || !errors.Is(result.HealthError, ErrHTTPStatus) {
					t.Fatal("health failure changed confirmed push result")
				}
			}
			if mode == "intake-deadline" {
				if err == nil || result.HealthError != nil || intakes.Load() != 1 {
					t.Fatal("intake deadline canceled independent heartbeat")
				}
			}
		})
	}
}
