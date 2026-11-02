package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const summaryFixture = `{"ok":true,"project":{"name":"Fixture","slug":"fixture","env":null,"url":null,"icon":null},"stats":{"runtimes":{"open":9007199254740993,"total":9007199254740993},"slow_queries":{"open":0,"total":0},"todos":{"open":0,"in_progress":0,"done":1,"total":1}},"recent":{"runtimes":[{"hash":"abcdef012345","exc_class":null,"exc_message":null,"count":9007199254740993,"last_seen":null,"status":"open"}],"slow_queries":[{"hash":"abcdef012345","sql_preview":null,"max_ms":2.67,"count":0,"last_seen":null,"status":"resolved","at_file":null,"at_line":null}],"todos":[{"id":"90071992547409931234567890","title":"Fixed task","status":"done","priority":"normal","created_at":null}]},"generated_at":"2026-11-02T00:00:00+00:00","future":"not projected"}`

func TestSummaryProjectionAndRequest(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var fields map[string]json.RawMessage
		if r.Method != "POST" || r.URL.Path != "/api/v1/summary" || json.NewDecoder(r.Body).Decode(&fields) != nil || len(fields) != 2 || rawString(fields["token"]) != fakeToken || string(fields["limit"]) != "5" {
			t.Error("request projection")
		}
		fmt.Fprint(w, summaryFixture)
	}))
	defer server.Close()
	c := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken})
	value, err := c.Summary(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if value.Stats.Runtimes.Open != 9007199254740993 || value.Recent.Todos[0].ID != "90071992547409931234567890" || value.Project.URL != nil || value.Recent.SlowQueries[0].Line != nil {
		t.Fatal("precision/null projection")
	}
	encoded, _ := json.Marshal(value)
	if strings.Contains(string(encoded), "future") {
		t.Fatal("unknown fields leaked")
	}
	for _, limit := range []int{0, 11, -1} {
		if _, err = c.Summary(context.Background(), limit); !errors.Is(err, ErrWire) {
			t.Fatal("invalid limit", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("invalid limits made HTTP")
	}
}
func TestSummaryMalformedAndSafeTransport(t *testing.T) {
	for _, data := range []string{
		strings.Replace(summaryFixture, `"open":9007199254740993`, `"open":null`, 1),
		strings.Replace(summaryFixture, `"max_ms":2.67`, `"max_ms":-1`, 1),
		strings.Replace(summaryFixture, `"at_line":null`, `"at_line":-1`, 1),
		strings.Replace(summaryFixture, `"exc_class":null,`, "", 1),
		strings.Replace(summaryFixture, `"todos":[`, `"todos":null,"unknown":[`, 1),
		strings.Replace(summaryFixture, `"count":9007199254740993`, `"count":1.5`, 1),
		strings.Replace(summaryFixture, `"id":"90071992547409931234567890"`, `"id":90071992547409931234567890`, 1),
		strings.Replace(summaryFixture, `"ok":true`, `"ok":true,"ok":true`, 1),
	} {
		if _, err := decodeSummary([]byte(data), 5); !errors.Is(err, ErrHTTPReceipt) {
			t.Fatal("accepted malformed summary")
		}
	}
	var requests atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(307)
		fmt.Fprint(w, fakeToken)
	}))
	defer server.Close()
	c := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken})
	_, err := c.Summary(context.Background(), 1)
	var failure *HTTPError
	if !errors.As(err, &failure) || failure.Operation != "summary" || !errors.Is(err, ErrHTTPStatus) || strings.Contains(err.Error(), fakeToken) || requests.Load() != 0 {
		t.Fatal("unsafe redirect", err)
	}
	low := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, summaryFixture) }))
	defer low.Close()
	c = httpClient(t, HTTPConfig{BaseURL: low.URL, Token: fakeToken, MaxResponseBytes: 32})
	if _, err = c.Summary(context.Background(), 1); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatal("response budget bypass", err)
	}
}

func TestSummaryInteractiveTimeoutAndRetry(t *testing.T) {
	var calls atomic.Int64
	borrowed := &http.Client{Timeout: 20 * time.Second, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := calls.Add(1)
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 4100*time.Millisecond || time.Until(deadline) < 900*time.Millisecond {
			t.Error("interactive timeout")
		}
		if n == 1 {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("fixed private cause")}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(summaryFixture))}, nil
	})}
	c := httpClient(t, HTTPConfig{BaseURL: "https://fixture.invalid", Token: fakeToken, Timeout: 20 * time.Second, Client: borrowed})
	if _, err := c.Summary(context.Background(), 5); err != nil || calls.Load() != 2 || borrowed.Timeout != 20*time.Second {
		t.Fatal("retry/shared mutation", err, calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Summary(ctx, 5); !errors.Is(err, context.Canceled) || calls.Load() != 2 {
		t.Fatal("caller cancellation", err, calls.Load())
	}
}
