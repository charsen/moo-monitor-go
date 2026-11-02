package cloudsync

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestExplicitOnceClientCountsDisconnectedRequests(t *testing.T) {
	batch := oneHTTPBatch(t)
	slow, input, now := slowSyncStore(t)
	if _, captured, err := slow.ReportSlowQuery(context.Background(), input, now); err != nil || !captured {
		t.Fatal(err)
	}
	slowBatch, err := slow.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"runtime", "slow", "discard-runtime", "discard-slow", "heartbeat", "mature-default-runtime"} {
		t.Run(operation, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				connection, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = connection.Close()
			}))
			defer server.Close()
			once := operation != "mature-default-runtime"
			client := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken, Timeout: time.Second, DisableRetries: once})
			if client.owned == nil || client.owned.DisableKeepAlives != once {
				t.Fatal("owned transport reuse policy drift")
			}
			switch operation {
			case "runtime", "mature-default-runtime":
				_, err = client.Send(context.Background(), batch)
			case "slow":
				_, err = client.SendSlow(context.Background(), slowBatch)
			case "discard-runtime":
				_, err = client.DiscardLocalNoise(context.Background())
			case "discard-slow":
				_, err = client.DiscardSlowLocalNoise(context.Background())
			case "heartbeat":
				err = client.Heartbeat(context.Background(), healthMeta())
			}
			if !errors.Is(err, ErrHTTPTransport) {
				t.Fatal("disconnect not preserved", err)
			}
			expected := int64(1)
			if !once {
				expected = 3
			}
			if calls.Load() != expected {
				t.Fatal("unexpected repeated request", operation, calls.Load())
			}
		})
	}
}
func TestExplicitOnceRequestHasNoReplayBody(t *testing.T) {
	var calls atomic.Int64
	client := httpClient(t, HTTPConfig{BaseURL: "https://once.invalid", Token: fakeToken, DisableRetries: true, Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.GetBody != nil {
			t.Error("explicit once request retained replay factory")
		}
		return nil, io.ErrUnexpectedEOF
	})}})
	if _, err := client.DiscardLocalNoise(context.Background()); !errors.Is(err, ErrHTTPTransport) || calls.Load() != 1 {
		t.Fatal("explicit loop retried borrowed transport", err, calls.Load())
	}
}
