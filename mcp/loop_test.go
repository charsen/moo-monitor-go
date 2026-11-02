package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type fixtureTools struct{ failure bool }

func (f fixtureTools) Instructions() string { return "fixed instructions" }
func (f fixtureTools) Definitions(context.Context) ([]ToolDefinition, error) {
	return []ToolDefinition{{Name: "fixture", InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`)}}, nil
}
func (f fixtureTools) Call(ctx context.Context, name string, args json.RawMessage) (ToolResult, error) {
	if name == "panic" {
		panic("fake-token-private")
	}
	if f.failure {
		return ToolResult{}, errors.New("fake-token-private")
	}
	return ToolResult{Content: []Content{{Type: "text", Text: "complete text"}, {Type: "image", Data: "Zml4ZWQ=", MimeType: "image/png"}}, StructuredContent: json.RawMessage(`{"value":1}`)}, nil
}
func run(t *testing.T, input string, tools Toolset, cfg Config) ([]map[string]json.RawMessage, string, error) {
	t.Helper()
	if cfg.ServerName == "" {
		cfg.ServerName = "fixture"
		cfg.Version = "development"
	}
	server, err := NewServer(tools, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out, diag bytes.Buffer
	err = server.Serve(context.Background(), io.NopCloser(strings.NewReader(input)), &out, &diag)
	var replies []map[string]json.RawMessage
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var v map[string]json.RawMessage
		if e := json.Unmarshal([]byte(line), &v); e != nil {
			t.Fatal("stdout pollution")
		}
		replies = append(replies, v)
	}
	return replies, diag.String(), err
}
func start(version string) string {
	return `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + version + `"}}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
}
func code(r map[string]json.RawMessage) int {
	var v struct{ Code int }
	json.Unmarshal(r["error"], &v)
	return v.Code
}
func TestLoopVersionsAndIDs(t *testing.T) {
	for _, version := range []string{newest, "2025-03-26", "2024-11-05", "unsupported"} {
		t.Run(version, func(t *testing.T) {
			input := start(version) + `{"jsonrpc":"2.0","id":9007199254740993123456789,"method":"tools/list"}` + "\n" + `{"jsonrpc":"2.0","id":1.234567890123456789e1234,"method":"tools/call","params":{"name":"fixture","arguments":{"id":123456789123456789}}}` + "\n"
			replies, _, err := run(t, input, fixtureTools{}, Config{})
			if err != nil || len(replies) != 3 {
				t.Fatal(err, len(replies))
			}
			want := version
			if version == "unsupported" {
				want = newest
			}
			var init struct{ ProtocolVersion string }
			json.Unmarshal(replies[0]["result"], &init)
			if init.ProtocolVersion != want {
				t.Fatal(init)
			}
			if string(replies[1]["id"]) != "9007199254740993123456789" || string(replies[2]["id"]) != "1.234567890123456789e1234" {
				t.Fatal("ID precision lost")
			}
			structured := want == newest
			if bytes.Contains(replies[1]["result"], []byte("outputSchema")) != structured || bytes.Contains(replies[2]["result"], []byte("structuredContent")) != structured {
				t.Fatal("version projection")
			}
			for _, word := range []string{"complete text", "image/png", "Zml4ZWQ="} {
				if !bytes.Contains(replies[2]["result"], []byte(word)) {
					t.Fatal("legacy content lost")
				}
			}
		})
	}
	for _, id := range []string{`"exact string"`, `null`, `-0`, `0.00`, `1e+1000`} {
		replies, _, err := run(t, `{"jsonrpc":"2.0","id":`+id+`,"method":"ping"}`+"\n", fixtureTools{}, Config{})
		if err != nil || string(replies[0]["id"]) != id {
			t.Fatal(id, err)
		}
	}
}
func TestLoopValidationAndLifecycle(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int
	}{
		{"[", -32700}, {string([]byte{255}), -32700}, {`[]`, -32600}, {`null`, -32600}, {`{"jsonrpc":"2.0","id":[],"method":"ping"}`, -32600}, {`{"jsonrpc":"2.0","id":1,"id":2,"method":"ping"}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"ping","params":null}`, -32602}, {`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, -32600}, {`{"jsonrpc":"2.0","id":1,"method":"fake-private-method"}`, -32601},
	} {
		r, diag, err := run(t, tc.line+"\n", fixtureTools{}, Config{})
		if err != nil || len(r) != 1 || code(r[0]) != tc.want {
			t.Fatalf("code want%d got%v %v", tc.want, r, err)
		}
		b, _ := json.Marshal(r)
		if bytes.Contains(b, []byte("fake-private")) || diag != "" {
			t.Fatal("unsafe error")
		}
	}
	input := `{"jsonrpc":"2.0","method":"unknown-notification"}` + "\n" + start(newest) + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fixture","arguments":[]}}` + "\n" + `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"panic"}}` + "\n"
	r, diag, err := run(t, input, fixtureTools{}, Config{})
	if err != nil || len(r) != 3 || code(r[1]) != -32602 || code(r[2]) != -32603 || string(r[2]["id"]) != "3" || strings.Contains(diag, "private") {
		t.Fatal(r, diag, err)
	}
	r, diag, err = run(t, start(newest)+`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fixture"}}`+"\n", fixtureTools{true}, Config{})
	if err != nil || code(r[1]) != -32603 || strings.Contains(diag, "private") {
		t.Fatal(r, diag, err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestLoopLimitsAndCancellation(t *testing.T) {
	_, _, err := run(t, strings.Repeat("x", 400)+"\n", fixtureTools{}, Config{MaxInputBytes: 256})
	if !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
	r, _, err := run(t, start(newest), fixtureTools{}, Config{ServerName: "fixture", Version: strings.Repeat("v", 300), MaxOutputBytes: 256})
	if err != nil || code(r[0]) != -32603 {
		t.Fatal(r, err)
	}
	server, _ := NewServer(fixtureTools{}, Config{ServerName: "fixture", Version: "dev"})
	if err := server.Serve(context.Background(), io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)), shortWriter{}, io.Discard); !errors.Is(err, ErrOutput) {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, reader, io.Discard, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("owned reader not canceled")
	}
}
func TestLoopProcessEOF(t *testing.T) {
	if os.Getenv("MOO_MCP_LOOP_HELPER") == "1" {
		server, _ := NewServer(fixtureTools{}, Config{ServerName: "fixture", Version: "dev"})
		if server.Serve(context.Background(), os.Stdin, os.Stdout, os.Stderr) != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLoopProcessEOF$")
	cmd.Env = append(os.Environ(), "MOO_MCP_LOOP_HELPER=1")
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":"process","method":"ping"}` + "\n")
	out, err := cmd.Output()
	if err != nil || !json.Valid(bytes.TrimSpace(out)) {
		t.Fatal("stdio process failed", err)
	}
}

func TestLoopEmptyText(t *testing.T) {
	b, err := json.Marshal(Content{Type: "text"})
	if err != nil || !bytes.Contains(b, []byte(`"text":""`)) {
		t.Fatal(string(b), err)
	}
}

type blockingTools struct {
	fixtureTools
	entered chan struct{}
}

func (f blockingTools) Call(ctx context.Context, _ string, _ json.RawMessage) (ToolResult, error) {
	close(f.entered)
	<-ctx.Done()
	return ToolResult{}, ctx.Err()
}
func TestLoopToolCancellation(t *testing.T) {
	tool := blockingTools{entered: make(chan struct{})}
	server, _ := NewServer(tool, Config{ServerName: "fixture", Version: "dev"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx, io.NopCloser(strings.NewReader(start(newest)+`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fixture"}}`+"\n")), io.Discard, io.Discard)
	}()
	select {
	case <-tool.entered:
	case <-time.After(time.Second):
		t.Fatal("tool not entered")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("tool cancellation blocked")
	}
}
func TestLoopOversizeID(t *testing.T) {
	replies, _, err := run(t, `{"jsonrpc":"2.0","id":"`+strings.Repeat("x", 600)+`","method":"ping"}`+"\n", fixtureTools{}, Config{MaxOutputBytes: 256})
	if !errors.Is(err, ErrOutput) || len(replies) != 0 {
		t.Fatal("unbounded fallback", err)
	}
}

type structuredOnly struct{ fixtureTools }

func (structuredOnly) Call(context.Context, string, json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: []Content{{Type: "image", Data: "Zml4ZWQ=", MimeType: "image/png"}}, StructuredContent: json.RawMessage(`{"answer":1234567890123456789}`)}, nil
}
func TestLoopStructuredFallback(t *testing.T) {
	for _, version := range []string{newest, "2025-03-26", "2024-11-05"} {
		r, _, err := run(t, start(version)+`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fixture"}}`+"\n", structuredOnly{}, Config{})
		if err != nil || len(r) != 2 {
			t.Fatal(err)
		}
		var result ToolResult
		if err := json.Unmarshal(r[1]["result"], &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Content) != 2 || result.Content[0].Type != "image" || result.Content[1].Text != `{"answer":1234567890123456789}` {
			t.Fatal("fallback lost content", result)
		}
		if (len(result.StructuredContent) > 0) != (version == newest) {
			t.Fatal("version structured")
		}
	}
}
func TestLoopDiagnosticShortWrite(t *testing.T) {
	server, _ := NewServer(fixtureTools{true}, Config{ServerName: "fixture", Version: "dev"})
	var out bytes.Buffer
	err := server.Serve(context.Background(), io.NopCloser(strings.NewReader(start(newest)+`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fixture"}}`+"\n")), &out, shortWriter{})
	if !errors.Is(err, ErrOutput) {
		t.Fatal(err)
	}
}
