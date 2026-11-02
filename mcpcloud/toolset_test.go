package mcpcloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/charsen/moo-go-monitor/cloudsync"
	"gitee.com/charsen/moo-go-monitor/mcp"
)

const todoID = "01JTESTABCDEFGHIJKLMNOPQRS"

func fixture(t *testing.T, fn func(http.ResponseWriter, *http.Request, map[string]json.RawMessage)) (*Toolset, *cloudsync.HTTPClient, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]json.RawMessage
		if r.Method != "POST" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&body) != nil || stringValue(body["token"]) != "fixed-mcp-token" {
			t.Error("request projection")
			w.WriteHeader(400)
			return
		}
		fn(w, r, body)
	}))
	t.Cleanup(server.Close)
	client, err := cloudsync.NewHTTPClient(cloudsync.HTTPConfig{BaseURL: server.URL, Token: "fixed-mcp-token", Timeout: 5 * time.Second, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	tools, err := New(client)
	if err != nil {
		t.Fatal(err)
	}
	return tools, client, &calls
}
func call(t *testing.T, tools *Toolset, name, args string) mcp.ToolResult {
	t.Helper()
	result, err := tools.Call(context.Background(), name, json.RawMessage(args))
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestSevenCloudTools(t *testing.T) {
	seen := map[string]int{}
	tools, _, calls := fixture(t, func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) {
		seen[r.URL.Path]++
		switch r.URL.Path {
		case "/api/v1/runtimes/list":
			if string(b["limit"]) != "2" || string(b["offset"]) != "3" {
				t.Error("list projection")
			}
			fmt.Fprint(w, `{"ok":true,"count":1,"offset":3,"has_more":true,"next_offset":4,"runtimes":[{"hash":"aaaaaaaaaaaa","count":9007199254740993}]}`)
		case "/api/v1/runtimes/get":
			if string(b["with_payload"]) != "false" {
				t.Error("payload default")
			}
			fmt.Fprint(w, `{"ok":true,"runtime":{"hash":"aaaaaaaaaaaa","markdown":"runtime discussion","payload":{"body":"never-default"}}}`)
		case "/api/v1/runtimes/resolve":
			if stringValue(b["note"]) != "fixed note" {
				t.Error("note projection")
			}
			fmt.Fprint(w, `{"ok":true,"already_resolved":true,"runtime":{"hash":"aaaaaaaaaaaa","status":"resolved","resolved_by":"existing owner"}}`)
		case "/api/v1/todos/list":
			fmt.Fprint(w, `{"ok":true,"count":0,"offset":0,"has_more":false,"next_offset":null,"todos":[]}`)
		case "/api/v1/todos/get":
			if len(b) != 2 {
				t.Error("with_network must stay local")
			}
			fmt.Fprintf(w, `{"ok":true,"todo":{"id":%q,"markdown":"safe source discussion","category":"backend_bug","context_requests":[{"method":"POST","status":500,"headers":{"token":"special-network-secret"},"body":"special-network-secret","responseBody":"special-network-secret"}],"context_errors":[{"message":"fixed JS error","raw":"special-error-secret"}],"events":[{"note":"discussion kept"}]}}`, todoID)
		case "/api/v1/todos/image":
			if stringValue(b["attachment_id"]) != "900719925474099312345" {
				t.Error("attachment precision")
			}
			fmt.Fprintf(w, `{"ok":true,"image":{"id":"900719925474099312345","todo_id":%q,"mime_type":"image/png","size_bytes":5,"data":"Zml4ZWQ="}}`, todoID)
		case "/api/v1/todos/status":
			fmt.Fprintf(w, `{"ok":true,"already_done":true,"todo":{"id":%q,"status":"done","done_note":"original"}}`, todoID)
		default:
			t.Error("unexpected path")
			w.WriteHeader(404)
		}
	})
	defs, err := tools.Definitions(context.Background())
	if err != nil || len(defs) != 7 {
		t.Fatal(err)
	}
	for _, d := range defs {
		if d.Annotations == nil || len(d.OutputSchema) == 0 {
			t.Fatal("missing contracts")
		}
	}
	cases := []struct{ name, args string }{{"list_open_runtimes", `{"limit":2,"offset":3}`}, {"get_runtime", `{"hash":"aaaaaaaaaaaa"}`}, {"resolve_runtime", `{"hash":"aaaaaaaaaaaa","note":"fixed note"}`}, {"list_open_todos", `{}`}, {"get_todo", `{"id":"` + todoID + `"}`}, {"get_todo_image", `{"id":"` + todoID + `","attachment_id":"900719925474099312345"}`}, {"update_todo_status", `{"id":"` + todoID + `","status":"done"}`}}
	for _, tc := range cases {
		result := call(t, tools, tc.name, tc.args)
		if result.IsError {
			t.Fatal(tc.name, result)
		}
		if len(result.Content) == 0 || len(result.StructuredContent) == 0 {
			t.Fatal("missing complete output")
		}
		all, _ := json.Marshal(result)
		switch tc.name {
		case "get_runtime":
			if bytes.Contains(all, []byte("never-default")) {
				t.Fatal("payload leak")
			}
		case "get_todo":
			for _, value := range []string{"safe source discussion", "discussion kept"} {
				if !bytes.Contains(all, []byte(value)) {
					t.Fatal("discussion lost")
				}
			}
			if bytes.Contains(all, []byte("special-")) {
				t.Fatal("default structured/text leak")
			}
		case "get_todo_image":
			if len(result.Content) != 2 || result.Content[1].Type != "image" {
				t.Fatal("private image lost")
			}
		}
	}
	if calls.Load() != 7 || len(seen) != 7 {
		t.Fatal("seven actual operations", calls.Load(), seen)
	}
	result := call(t, tools, "get_todo", `{"id":"`+todoID+`","with_network_payloads":true}`)
	all, _ := json.Marshal(result)
	if result.IsError || !bytes.Contains(all, []byte("special-network-secret")) || bytes.Contains(all, []byte("special-error-secret")) {
		t.Fatal("opt-in filtering")
	}
}
func TestInvalidArgumentsNoHTTP(t *testing.T) {
	tools, _, calls := fixture(t, func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) {
		t.Error("invalid arguments reached HTTP")
	})
	for _, tc := range []struct{ name, args string }{
		{"list_open_runtimes", `{"limit":0}`}, {"list_open_runtimes", `{"limit":51}`}, {"list_open_todos", `{"offset":1e5}`}, {"list_open_todos", `{"offset":-1}`}, {"list_open_todos", `{"status":"resolved"}`}, {"get_runtime", `{"hash":"AAAAAAAAAAAA"}`}, {"get_runtime", `{"hash":"aaaaaaaaaaaa","with_payload":[]}`}, {"get_todo", `{"id":12345678901234567890}`}, {"get_todo", `{"id":"` + todoID + `","id":"` + todoID + `"}`}, {"get_todo", `{"id":"` + todoID + `","token":"fake"}`}, {"get_todo_image", `{"id":"` + todoID + `","attachment_id":"https://external.invalid"}`}, {"resolve_runtime", `{"hash":"aaaaaaaaaaaa","note":{}}`}, {"update_todo_status", `{"id":"` + todoID + `","status":"deleted"}`}, {"unknown-tool", `{}`},
	} {
		if result := call(t, tools, tc.name, tc.args); !result.IsError {
			t.Fatal("invalid accepted", tc.name)
		}
	}
	if calls.Load() != 0 {
		t.Fatal(calls.Load())
	}
}
func TestPaginationAndSafeErrors(t *testing.T) {
	for _, body := range []string{`{"ok":true,"runtimes":[]}`, `{"ok":true,"runtimes":[],"offset":0}`, `{"ok":true,"runtimes":[],"offset":0,"has_more":true,"next_offset":0}`, `{"ok":true,"runtimes":[],"offset":0,"has_more":false,"next_offset":1}`, `{"ok":true,"runtimes":[],"offset":0,"has_more":false,"next_offset":null,"count":1}`, `{"ok":true,"ok":true,"runtimes":[]}`, `{"ok":false,"error":"fixed-mcp-token arbitrary-private-cause"}`} {
		t.Run("receipt", func(t *testing.T) {
			tools, _, _ := fixture(t, func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) { fmt.Fprint(w, body) })
			result := call(t, tools, "list_open_runtimes", `{}`)
			if body == `{"ok":true,"runtimes":[]}` {
				if result.IsError || !bytes.Contains(result.StructuredContent, []byte(`"has_more":null`)) {
					t.Fatal("legacy page")
				}
				result = call(t, tools, "list_open_runtimes", `{"offset":1}`)
			}
			if !result.IsError {
				t.Fatal("bad paging accepted")
			}
			out, _ := json.Marshal(result)
			if bytes.Contains(out, []byte("token")) || bytes.Contains(out, []byte("private")) {
				t.Fatal("unsafe failure")
			}
		})
	}
}
func TestPrivateImagesAndResponseCap(t *testing.T) {
	for _, tc := range []struct {
		data, mime string
		ok         bool
	}{{"Zml4ZWQ=", "image/png", true}, {"Zml4ZWQ=", "image/jpeg", true}, {"Zml4ZWQ=", "image/webp", true}, {"Zml4ZWQ=", "image/gif", true}, {"not-base64!", "image/png", false}, {"Zml4ZWQ=", "image/svg+xml", false}, {base64.StdEncoding.EncodeToString(make([]byte, (2<<20)+1)), "image/png", false}} {
		t.Run(tc.mime, func(t *testing.T) {
			_, client, _ := fixture(t, func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) {
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "image": map[string]any{"id": "1", "todo_id": todoID, "mime_type": tc.mime, "data": tc.data}})
			})
			_, err := client.FetchTodoImage(context.Background(), todoID, "1")
			if (err == nil) != tc.ok {
				t.Fatal("image validation", err)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 1024)) }))
	defer server.Close()
	client, err := cloudsync.NewHTTPClient(cloudsync.HTTPConfig{BaseURL: server.URL, Token: "fixed", MaxResponseBytes: 128})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	_, err = client.FetchTodoImage(context.Background(), todoID, "1")
	if !errors.Is(err, cloudsync.ErrResponseTooLarge) {
		t.Fatal("explicit cap bypassed", err)
	}
}
func TestCloudToolsetRealStdio(t *testing.T) {
	tools, _, _ := fixture(t, func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) {
		fmt.Fprintf(w, `{"ok":true,"image":{"id":"1","todo_id":%q,"mime_type":"image/png","data":"Zml4ZWQ="}}`, todoID)
	})
	for _, version := range []string{"2025-06-18", "2025-03-26", "2024-11-05"} {
		server, err := mcp.NewServer(tools, mcp.Config{ServerName: "fixture", Version: "development"})
		if err != nil {
			t.Fatal(err)
		}
		input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q}}`+"\n"+`{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"+`{"jsonrpc":"2.0","id":90071992547409931234,"method":"tools/call","params":{"name":"get_todo_image","arguments":{"id":%q,"attachment_id":"1"}}}`+"\n", version, todoID)
		var out, diag bytes.Buffer
		if err = server.Serve(context.Background(), io.NopCloser(strings.NewReader(input)), &out, &diag); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), `"type":"image"`) || !strings.Contains(out.String(), `90071992547409931234`) || strings.Contains(out.String(), "fixed-mcp-token") || diag.Len() != 0 {
			t.Fatal("stdio output")
		}
		if strings.Contains(out.String(), "structuredContent") != (version == "2025-06-18") {
			t.Fatal("legacy version")
		}
	}
}

func TestStatusReceiptAndBooleanCompatibility(t *testing.T) {
	for _, tc := range []struct {
		requested, actual string
		done, progress    bool
		ok                bool
	}{{"in_progress", "done", true, false, true}, {"in_progress", "done", false, false, false}, {"done", "in_progress", false, false, false}, {"in_progress", "in_progress", false, true, true}, {"done", "done", true, true, false}} {
		tools, _, _ := fixture(t, func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) {
			fmt.Fprintf(w, `{"ok":true,"already_done":%t,"already_in_progress":%t,"error":"private-error-not-output","todo":{"id":%q,"status":%q}}`, tc.done, tc.progress, todoID, tc.actual)
		})
		result := call(t, tools, "update_todo_status", `{"id":"`+todoID+`","status":"`+tc.requested+`"}`)
		if result.IsError == tc.ok {
			t.Fatal("wrong status classified", tc, result)
		}
		out, _ := json.Marshal(result)
		if bytes.Contains(out, []byte("private-error")) {
			t.Fatal("extra Cloud error leaked")
		}
	}
	tools, _, _ := fixture(t, func(w http.ResponseWriter, r *http.Request, b map[string]json.RawMessage) {
		if string(b["with_payload"]) != "false" {
			t.Error("unknown boolean string escalated payload")
		}
		fmt.Fprint(w, `{"ok":true,"runtime":{"hash":"aaaaaaaaaaaa","markdown":"fixed"}}`)
	})
	for _, raw := range []string{`"false"`, `"unrecognized"`} {
		if result := call(t, tools, "get_runtime", `{"hash":"aaaaaaaaaaaa","with_payload":`+raw+`}`); result.IsError {
			t.Fatal(result)
		}
	}
}
