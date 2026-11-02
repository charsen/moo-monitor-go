package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHeartbeatGoMetadataProjection(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body struct {
			Meta  map[string]any `json:"meta"`
			Token string         `json:"token"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Token != fakeToken || body.Meta["go_version"] != "go1.27.3" || body.Meta["goravel_version"] != "v1.18.0" || len(body.Meta) != 12 {
			t.Error("optional Go metadata wire")
		}
		for _, key := range []string{"php_version", "laravel_version"} {
			if _, present := body.Meta[key]; present {
				t.Error("fake PHP metadata")
			}
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	c := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken})
	meta := healthMeta()
	meta.GoVersion = "go1.27.3"
	meta.GoravelVersion = "v1.18.0"
	if err := c.Heartbeat(context.Background(), meta); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{string([]byte{255}), strings.Repeat("x", 65)} {
		bad := meta
		bad.GoVersion = value
		if err := c.Heartbeat(context.Background(), bad); !errors.Is(err, ErrWire) {
			t.Fatal("invalid Go metadata", err)
		}
		bad = meta
		bad.GoravelVersion = value
		if err := c.Heartbeat(context.Background(), bad); !errors.Is(err, ErrWire) {
			t.Fatal("invalid Goravel metadata", err)
		}
	}
	if requests.Load() != 1 {
		t.Fatal("invalid metadata made request")
	}
}
