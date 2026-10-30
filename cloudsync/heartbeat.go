package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"
)

const heartbeatPath = "api/v1/heartbeat"

// HeartbeatMeta contains only fields currently accepted by Cloud. Platform versions
// belong to local diagnostics until the consumer explicitly supports Go metadata.
type HeartbeatMeta struct {
	SDKVersion     string `json:"sdk_version"`
	AppEnv         string `json:"app_env"`
	AppName        string `json:"app_name"`
	CloudEnabled   bool   `json:"cloud_enabled"`
	RuntimeEnabled bool   `json:"runtime_enabled"`
	SlowSQLEnabled bool   `json:"slow_sql_enabled"`
	PushRuntimes   bool   `json:"push_runtimes"`
	PushSlowSQL    bool   `json:"push_slow_sql"`
	Schedule       bool   `json:"schedule"`
}

type heartbeatWireMeta struct {
	SDK string `json:"sdk"`
	HeartbeatMeta
}

type Heartbeater interface {
	Heartbeat(context.Context, HeartbeatMeta) error
}

func (c *HTTPClient) Heartbeat(ctx context.Context, meta HeartbeatMeta) error {
	if !validWireString(meta.SDKVersion) || !validWireString(meta.AppEnv) || !validWireString(meta.AppName) {
		return &HTTPError{Operation: "heartbeat", category: ErrWire}
	}
	body, err := json.Marshal(struct {
		Token string            `json:"token"`
		Meta  heartbeatWireMeta `json:"meta"`
	}{c.token, heartbeatWireMeta{"moo-go-monitor", meta}})
	if err != nil {
		return &HTTPError{Operation: "heartbeat", category: ErrWire}
	}
	// A per-operation client value shares the mature transport, not mutable timeout state.
	operation := *c
	client := *c.client
	client.Timeout = max(time.Second, min(client.Timeout, 4*time.Second))
	operation.client = &client
	data, status, attempt, err := operation.post(ctx, heartbeatPath, body, 2, 100*time.Millisecond)
	if err != nil {
		var failure *HTTPError
		if errors.As(err, &failure) {
			copy := *failure
			copy.Operation = "heartbeat"
			return &copy
		}
		return &HTTPError{Operation: "heartbeat", category: ErrHTTPTransport}
	}
	var fields map[string]json.RawMessage
	if !utf8.Valid(data) || decodeJSON(data, &fields) != nil || fields == nil || !bytes.Equal(bytes.TrimSpace(fields["ok"]), []byte("true")) {
		return &HTTPError{Operation: "heartbeat", Status: status, Attempt: attempt, category: ErrHTTPReceipt}
	}
	return nil
}
