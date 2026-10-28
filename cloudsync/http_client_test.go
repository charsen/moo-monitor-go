package cloudsync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/mask"
	"gitee.com/charsen/moo-go-monitor/records"
)

const fakeToken = "FixedFakeProjectToken0123456789abcdef"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
func httpClient(t *testing.T, config HTTPConfig) *HTTPClient {
	t.Helper()
	client, err := NewHTTPClient(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}
func oneHTTPBatch(t *testing.T) []buffer.Snapshot {
	t.Helper()
	store := makeStore(t, privateRoot(t), "")
	snapshot := event(1)
	snapshot.Request.Method = "POST"
	snapshot.Payload = mask.OrderedObject{{Key: "z", Value: int64(9007199254740993)}, {Key: "a", Value: nil}}
	snapshot.Exception.Previous = append(snapshot.Exception.Previous, records.Previous{Class: "FixturePrevious", Message: "fixed fake", File: "/fixture/previous.go", Line: 7})
	if _, err := store.Record(context.Background(), snapshot, clock); err != nil {
		t.Fatal(err)
	}
	batch, err := store.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return batch
}

func TestHTTPPostWireAndDiscard(t *testing.T) {
	batch := oneHTTPBatch(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "" {
			t.Error("HTTP method/headers")
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(data, &fields); err != nil {
			t.Error(err)
		}
		var token string
		json.Unmarshal(fields["token"], &token)
		if token != fakeToken || len(fields) != 2 {
			t.Error("body auth/whitelist")
		}
		switch r.URL.Path {
		case "/prefix/" + runtimeIntakePath:
			var records []json.RawMessage
			if err = json.Unmarshal(fields["records"], &records); err != nil || len(records) != 1 {
				t.Error("records", err)
			}
			expected, _ := json.Marshal(batch[0].Record)
			var got, want any
			decoder := json.NewDecoder(bytes.NewReader(records[0]))
			decoder.UseNumber()
			decoder.Decode(&got)
			decoder = json.NewDecoder(bytes.NewReader(expected))
			decoder.UseNumber()
			decoder.Decode(&want)
			if !reflect.DeepEqual(got, want) {
				t.Error("full Record changed")
			}
			row := got.(map[string]any)
			if row["Digest"] != nil || row["digest"] != nil || row["bucket"] != nil || row["Record"] != nil {
				t.Error("Snapshot wrapper leaked")
			}
			if !bytes.Contains(records[0], []byte("9007199254740993")) || !bytes.Contains(records[0], []byte(`"a":null`)) {
				t.Error("numeric/null wire")
			}
			io.WriteString(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0}`)
		case "/prefix/" + runtimeDiscardPath:
			if string(fields["env"]) != `"local"` {
				t.Error("discard env")
			}
			io.WriteString(w, `{"ok":true,"deleted":2}`)
		default:
			t.Error("unexpected fixed path", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := httpClient(t, HTTPConfig{BaseURL: server.URL + "/prefix/", Token: fakeToken})
	if _, err := client.Send(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	if deleted, err := client.DiscardLocalNoise(context.Background()); err != nil || deleted != 2 {
		t.Fatal(deleted, err)
	}
	if requests != 2 {
		t.Fatal("request count", requests)
	}
}

func TestHTTPWireSourceFixture(t *testing.T) {
	data, err := os.ReadFile("../docs/contracts/fixtures/cloud-json.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name string
			Spec struct {
				Type, Base64 string
				Value        json.RawMessage
				Entries      []struct {
					Key   string
					Value json.RawMessage
				}
			} `json:"input_spec"`
			JSON      *string `json:"json"`
			ErrorCode int     `json:"error_code"`
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			var value any
			switch tc.Spec.Type {
			case "array":
				value = []any{}
			case "object":
				object := mask.OrderedObject{}
				for _, entry := range tc.Spec.Entries {
					var item any
					json.Unmarshal(entry.Value, &item)
					object = append(object, mask.Entry{Key: entry.Key, Value: item})
				}
				value = object
			case "int64":
				var text string
				json.Unmarshal(tc.Spec.Value, &text)
				value, err = strconv.ParseInt(text, 10, 64)
				if err != nil {
					t.Fatal(err)
				}
			case "bytes":
				raw, err := base64.StdEncoding.DecodeString(tc.Spec.Base64)
				if err != nil {
					t.Fatal(err)
				}
				value = string(raw)
			case "nonfinite":
				var text string
				json.Unmarshal(tc.Spec.Value, &text)
				value = math.Inf(1)
				if text == "nan" {
					value = math.NaN()
				}
			default:
				if len(tc.Spec.Value) > 0 {
					json.Unmarshal(tc.Spec.Value, &value)
				}
			}
			validation := validateWireValue(value, 0)
			if tc.JSON == nil {
				if validation == nil {
					t.Fatal("native PHP encoding failure accepted")
				}
				return
			}
			if validation != nil {
				t.Fatal(validation)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var actual, expected any
			decoder := json.NewDecoder(bytes.NewReader(encoded))
			decoder.UseNumber()
			decoder.Decode(&actual)
			decoder = json.NewDecoder(strings.NewReader(*tc.JSON))
			decoder.UseNumber()
			decoder.Decode(&expected)
			if !reflect.DeepEqual(actual, expected) {
				t.Fatal("native JSON semantics differ", string(encoded), *tc.JSON)
			}
		})
	}
}

func TestHTTPRetriesAndSafeErrors(t *testing.T) {
	batch := oneHTTPBatch(t)
	for _, mode := range []string{"connection", "arbitrary", "http-status", "discard"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			sentinel := errors.New("fixed fake failure " + fakeToken)
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				switch mode {
				case "http-status":
					return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(`{"error":"` + fakeToken + `"}`)), Header: make(http.Header)}, nil
				case "arbitrary":
					return nil, sentinel
				default:
					return nil, &net.OpError{Op: "dial", Net: "tcp", Err: sentinel}
				}
			})
			borrowed := &http.Client{Transport: transport}
			client := httpClient(t, HTTPConfig{BaseURL: "http://fixture.invalid", Token: fakeToken, Client: borrowed})
			var err error
			if mode == "discard" {
				_, err = client.DiscardLocalNoise(context.Background())
			} else {
				_, err = client.Send(context.Background(), batch)
			}
			wanted := 1
			if mode == "connection" {
				wanted = 3
			}
			if mode == "discard" {
				wanted = 2
			}
			if calls != wanted || err == nil || strings.Contains(err.Error(), fakeToken) {
				t.Fatal("retry/error safety", calls, err)
			}
			if mode != "http-status" && !errors.Is(err, sentinel) {
				t.Fatal("cause not identifiable")
			}
			if mode == "http-status" && !errors.Is(err, ErrHTTPStatus) {
				t.Fatal("status classification")
			}
		})
	}
}

func TestHTTPContextAndLimits(t *testing.T) {
	batch := oneHTTPBatch(t)
	calls := 0
	client := httpClient(t, HTTPConfig{BaseURL: "http://fixture.invalid", Token: fakeToken, Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: io.EOF}
	})}})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	start := time.Now()
	if _, err := client.Send(ctx, batch); !errors.Is(err, context.Canceled) {
		t.Fatal("backoff cancellation", err)
	}
	if calls != 1 || time.Since(start) > 150*time.Millisecond {
		t.Fatal("cancel kept retrying", calls)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(150 * time.Millisecond):
			io.WriteString(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0}`)
		}
	}))
	defer server.Close()
	timed := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken, Timeout: 20 * time.Millisecond})
	if _, err := timed.Send(context.Background(), batch); err == nil || !errors.Is(err, ErrHTTPTransport) {
		t.Fatal("timeout", err)
	}
	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, strings.Repeat("x", 200)) }))
	defer large.Close()
	limited := httpClient(t, HTTPConfig{BaseURL: large.URL, Token: fakeToken, MaxResponseBytes: 32})
	if _, err := limited.Send(context.Background(), batch); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatal("response limit", err)
	}
	for _, wire := range []string{string(append([]byte(`{"ok":true,"deleted":1,"error":"`), append([]byte{0xff}, []byte(`"}`)...)...)), `{"ok":true,"deleted":null}`, `{"ok":true,"deleted":1.0}`, `{"ok":true,"deleted":-1}`, `{"ok":true}`, `{"ok":"true","deleted":1}`} {
		if _, err := decodeDiscard([]byte(wire)); err == nil {
			t.Fatal("bad discard receipt")
		}
	}
}

func TestHTTPRedirectAndTLS(t *testing.T) {
	batch := oneHTTPBatch(t)
	for _, code := range []int{307, 308} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			var second atomic.Int64
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				second.Add(1)
				io.WriteString(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0}`)
			}))
			defer destination.Close()
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", destination.URL)
				w.WriteHeader(code)
			}))
			defer source.Close()
			client := httpClient(t, HTTPConfig{BaseURL: source.URL, Token: fakeToken})
			if _, err := client.Send(context.Background(), batch); !errors.Is(err, ErrHTTPStatus) {
				t.Fatal("redirect accepted", err)
			}
			if second.Load() != 0 {
				t.Fatal("token reached redirect host")
			}
		})
	}
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0}`)
	}))
	defer tlsServer.Close()
	defaultTLS := httpClient(t, HTTPConfig{BaseURL: tlsServer.URL, Token: fakeToken})
	if _, err := defaultTLS.Send(context.Background(), batch); err == nil {
		t.Fatal("self-signed cert accepted by default")
	}
	borrowed := tlsServer.Client()
	originalTimeout := borrowed.Timeout
	originalRedirect := borrowed.CheckRedirect
	trusted := httpClient(t, HTTPConfig{BaseURL: tlsServer.URL, Token: fakeToken, Client: borrowed})
	if _, err := trusted.Send(context.Background(), batch); err != nil {
		t.Fatal("explicit trust", err)
	}
	if borrowed.Timeout != originalTimeout || reflect.ValueOf(borrowed.CheckRedirect) != reflect.ValueOf(originalRedirect) {
		t.Fatal("borrowed client mutated")
	}
}

func TestHTTPNoConfigOrBadWire(t *testing.T) {
	for _, config := range []HTTPConfig{{}, {BaseURL: "http://fixture.invalid"}, {BaseURL: "http://user:" + fakeToken + "@fixture.invalid", Token: fakeToken}, {BaseURL: "http://fixture.invalid?token=" + fakeToken, Token: fakeToken}, {BaseURL: "http://fixture.invalid#" + fakeToken, Token: fakeToken}, {BaseURL: "http://fixture.invalid/#", Token: fakeToken}, {BaseURL: "file:///tmp/fake", Token: fakeToken}} {
		if client, err := NewHTTPClient(config); err == nil || client != nil || strings.Contains(err.Error(), fakeToken) {
			t.Fatal("unsafe configuration error", err)
		}
	}
	requests := 0
	client := httpClient(t, HTTPConfig{BaseURL: "http://fixture.invalid", Token: fakeToken, Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { requests++; return nil, io.EOF })}})
	for _, kind := range []string{"trace", "metadata", "nonfinite", "unknown"} {
		batch := oneHTTPBatch(t)
		switch kind {
		case "trace":
			batch[0].Record.Trace.Full = string([]byte{0xff})
		case "metadata":
			batch[0].Record.Meta["bad"] = string([]byte{0xff})
		case "nonfinite":
			batch[0].Record.Payload = math.Inf(1)
		case "unknown":
			batch[0].Record.Payload = wireTrap{}
		}
		if _, err := client.Send(context.Background(), batch); !errors.Is(err, ErrWire) {
			t.Fatal("unsafe wire accepted", kind, err)
		}
	}
	if requests != 0 {
		t.Fatal("bad wire made requests", requests)
	}
}

type wireTrap struct{}

func (wireTrap) MarshalJSON() ([]byte, error) { panic("arbitrary marshaler must not run") }

func TestHTTPAttemptTimeouts(t *testing.T) {
	batch := oneHTTPBatch(t)
	for _, mode := range []string{"then-success", "exhausted", "caller-deadline"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
				attempt := calls.Add(1)
				if mode == "then-success" && attempt > 1 {
					io.WriteString(w, `{"ok":true,"saved":1,"filtered":0,"skipped":0}`)
					return
				}
				select {
				case <-r.Context().Done():
				case <-time.After(200 * time.Millisecond):
				}
			}))
			defer server.Close()
			client := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken, Timeout: 30 * time.Millisecond})
			ctx := context.Background()
			cancel := func() {}
			if mode == "caller-deadline" {
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
			}
			defer cancel()
			_, err := client.Send(ctx, batch)
			switch mode {
			case "then-success":
				if err != nil || calls.Load() != 2 {
					t.Fatal("per-attempt timeout did not retry", calls.Load(), err)
				}
			case "exhausted":
				var classified *HTTPError
				if !errors.As(err, &classified) || classified.Attempt != 3 || calls.Load() != 3 {
					t.Fatal("timeout attempt cap", calls.Load(), err)
				}
			case "caller-deadline":
				if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
					t.Fatal("caller deadline retried", calls.Load(), err)
				}
			}
		})
	}
}

func TestHTTPSyncMixedReceipts(t *testing.T) {
	store := makeStore(t, privateRoot(t), "")
	for line := 1; line <= 4; line++ {
		add(t, store, line, clock.Add(time.Duration(line)*time.Second))
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Token   string           `json:"token"`
			Records []records.Record `json:"records"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Token != fakeToken || r.URL.Path != "/"+runtimeIntakePath {
			t.Error("HTTP Sync authentication/path")
		}
		receipt := Receipt{}
		for index, record := range body.Records {
			item := Item{Index: index, Hash: record.Hash, Status: "saved"}
			if calls == 1 {
				switch record.Exception.Line {
				case 2:
					item.Status = "filtered"
					item.Reason = text("ingest_filter")
				case 3:
					item.Status = "skipped"
					item.Retryable = true
					item.Reason = text("upsert_failed")
				case 4:
					item.Status = "skipped"
					item.Reason = text("invalid_record")
				}
			} else if len(body.Records) != 1 || record.Exception.Line != 3 {
				t.Error("HTTP retry did not isolate failed hole")
			}
			switch item.Status {
			case "saved":
				receipt.Saved++
			case "filtered":
				receipt.Filtered++
			case "skipped":
				receipt.Skipped++
			}
			receipt.Results = append(receipt.Results, item)
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": receipt.Saved, "filtered": receipt.Filtered, "skipped": receipt.Skipped, "results": receipt.Results})
	}))
	defer server.Close()
	client := httpClient(t, HTTPConfig{BaseURL: server.URL, Token: fakeToken})
	syncer := makeSync(t, store, client)
	if result, err := syncer.Sync(context.Background()); !errors.Is(err, ErrPending) || result.Confirmed != 3 || result.Pending != 1 {
		t.Fatal("HTTP mixed batch", result, err)
	}
	syncer.Close()
	syncer = makeSync(t, store, client)
	if result, err := syncer.Sync(context.Background()); err != nil || result.Changed != 1 || result.Confirmed != 1 {
		t.Fatal("HTTP failure-hole retry/reopen", result, err)
	}
	if _, err := syncer.Sync(context.Background()); err != nil || calls != 2 {
		t.Fatal("HTTP anchors repeated", calls, err)
	}
}
