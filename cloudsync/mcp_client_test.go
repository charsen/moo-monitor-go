package cloudsync

import (
	"context"
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

type mcpTransport func(*http.Request) (*http.Response, error)

func (f mcpTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMCPRedirectAndSafeFailure(t *testing.T) {
	var leaked atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer other.Close()
	for _, status := range []int{307, 308} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", other.URL)
			w.WriteHeader(status)
			fmt.Fprint(w, "fixed-private-token raw error")
		}))
		client, err := NewHTTPClient(HTTPConfig{BaseURL: server.URL, Token: "fixed-private-token"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.FetchTodos(context.Background(), MCPListQuery{Limit: 20})
		client.CloseIdleConnections()
		server.Close()
		if !errors.Is(err, ErrHTTPStatus) || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), server.URL) {
			t.Fatal("unsafe redirect error", err)
		}
	}
	if leaked.Load() != 0 {
		t.Fatal("token forwarded")
	}
}
func TestMCPInteractiveTransport(t *testing.T) {
	var calls atomic.Int64
	borrowed := &http.Client{Transport: mcpTransport(func(r *http.Request) (*http.Response, error) {
		n := calls.Add(1)
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 4100*time.Millisecond || time.Until(deadline) < 900*time.Millisecond {
			t.Error("interactive clamp")
		}
		if n == 1 {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("fixed secret cause")}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"todos":[],"count":0,"offset":0,"has_more":false,"next_offset":null}`)), Header: make(http.Header)}, nil
	})}
	client, err := NewHTTPClient(HTTPConfig{BaseURL: "https://fixed.invalid", Token: "fixed-private-token", Timeout: 20 * time.Second, Client: borrowed})
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.FetchTodos(context.Background(), MCPListQuery{Limit: 20})
	if err != nil || len(page.Rows) != 0 || calls.Load() != 2 {
		t.Fatal("retry", err, calls.Load())
	}
	if client.client.Timeout != 20*time.Second || borrowed.Timeout != 0 {
		t.Fatal("shared client mutated")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.FetchTodos(ctx, MCPListQuery{Limit: 20})
	if !errors.Is(err, context.Canceled) || calls.Load() != 2 {
		t.Fatal("caller cancel retried", err)
	}
	client, _ = NewHTTPClient(HTTPConfig{BaseURL: "https://fixed.invalid", Token: "fixed", Timeout: 100 * time.Millisecond, Client: borrowed})
	calls.Store(1)
	if _, err = client.FetchTodos(context.Background(), MCPListQuery{Limit: 20}); err != nil {
		t.Fatal(err)
	}
}
