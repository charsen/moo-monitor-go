// Package mcpcloud binds the seven Cloud operations to the independent MCP loop.
package mcpcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/cloudsync"
	"gitee.com/charsen/moo-monitor-go/mcp"
)

type Client interface {
	FetchRuntimes(context.Context, cloudsync.MCPListQuery) (cloudsync.MCPPage, error)
	FetchRuntime(context.Context, string, bool) (cloudsync.MCPObject, error)
	ResolveRuntime(context.Context, string, *string, *string) (cloudsync.MCPObject, error)
	FetchTodos(context.Context, cloudsync.MCPListQuery) (cloudsync.MCPPage, error)
	FetchTodo(context.Context, string) (cloudsync.MCPObject, error)
	FetchTodoImage(context.Context, string, string) (cloudsync.MCPImage, error)
	UpdateTodoStatus(context.Context, string, string, *string, *string) (cloudsync.MCPObject, error)
}
type Toolset struct{ client Client }

func New(client Client) (*Toolset, error) {
	if client == nil {
		return nil, cloudsync.ErrHTTPConfig
	}
	return &Toolset{client}, nil
}
func (*Toolset) Instructions() string {
	return "先list并按next_offset翻页，再get完整上下文。runtime修复并验证后才resolve；Todo先in_progress认领、修复并验证后才done；有ai_readable图片时按attachment_id读现场。状态工具有真实云副作用，调用者必须先授权，不因工具可用就自动执行。敏感payload默认不包含，确有需要才显式开启。"
}
func (*Toolset) Definitions(context.Context) ([]mcp.ToolDefinition, error) {
	str := func() map[string]any { return map[string]any{"type": "string"} }
	boolean := map[string]any{"type": "boolean"}
	hash := map[string]any{"type": "string", "pattern": "^[a-f0-9]{12}$"}
	id := map[string]any{"type": "string", "pattern": "^[a-zA-Z0-9]{20,32}$"}
	listProps := func(statuses []string) map[string]any {
		return map[string]any{"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}, "offset": map[string]any{"type": "integer", "minimum": 0, "default": 0}, "status": map[string]any{"type": "string", "enum": statuses}}
	}
	defs := []mcp.ToolDefinition{
		definition("list_open_runtimes", "列出项目runtime；按next_offset翻页。", listProps([]string{"open", "in_progress", "resolved"}), nil, listSchema("runtimes"), false),
		definition("get_runtime", "查看完整runtime；请求payload默认关闭。", map[string]any{"hash": hash, "with_payload": boolean}, []string{"hash"}, detailSchema("runtime"), false),
		definition("resolve_runtime", "仅在授权且修复验证后标记resolved；既有解决记录不覆盖。", map[string]any{"hash": hash, "note": str(), "resolved_by": str()}, []string{"hash"}, statusSchema("hash"), true),
		definition("list_open_todos", "列出待办；按next_offset翻页。", listProps([]string{"open", "in_progress", "done"}), nil, listSchema("todos"), false),
		definition("get_todo", "查看待办/讨论/附件；网络headers/body默认不含。", map[string]any{"id": id, "with_network_payloads": boolean}, []string{"id"}, detailSchema("todo"), false),
		definition("get_todo_image", "经项目私有接口按id+attachment_id读单张图片；不访问外部URL。", map[string]any{"id": id, "attachment_id": map[string]any{"type": "string", "pattern": "^[0-9]{1,32}$"}}, []string{"id", "attachment_id"}, detailSchema("attachment"), false),
		definition("update_todo_status", "授权后in_progress认领；验证完成后才done，保既有完成记录。", map[string]any{"id": id, "status": map[string]any{"type": "string", "enum": []string{"in_progress", "done"}}, "note": str(), "by": str()}, []string{"id", "status"}, statusSchema("id"), true),
	}
	return defs, nil
}
func definition(name, description string, properties map[string]any, required []string, output any, write bool) mcp.ToolDefinition {
	input := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		input["required"] = required
	}
	in, _ := json.Marshal(input)
	out, _ := json.Marshal(output)
	readOnly, open, destructive, idempotent := !write, true, false, true
	annotations := &mcp.Annotations{Title: name, ReadOnlyHint: &readOnly, OpenWorldHint: &open}
	if write {
		annotations.DestructiveHint = &destructive
		annotations.IdempotentHint = &idempotent
	}
	return mcp.ToolDefinition{Name: name, Description: description, InputSchema: in, OutputSchema: out, Annotations: annotations}
}
func detailSchema(key string) any {
	return map[string]any{"type": "object", "properties": map[string]any{key: map[string]any{"type": "object"}}, "required": []string{key}}
}
func statusSchema(key string) any {
	return map[string]any{"type": "object", "properties": map[string]any{key: map[string]any{"type": "string"}, "status": map[string]any{"type": "string"}}, "required": []string{key, "status"}}
}
func listSchema(key string) any {
	return map[string]any{"type": "object", "properties": map[string]any{key: map[string]any{"type": "array", "items": map[string]any{"type": "object"}}, "count": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}, "pagination_supported": map[string]any{"type": "boolean"}, "has_more": map[string]any{"type": []string{"boolean", "null"}}, "next_offset": map[string]any{"type": []string{"integer", "null"}}}, "required": []string{key, "count", "offset", "pagination_supported", "has_more", "next_offset"}}
}

func (t *Toolset) Call(ctx context.Context, name string, raw json.RawMessage) (mcp.ToolResult, error) {
	allowed := map[string][]string{"list_open_runtimes": {"limit", "offset", "status"}, "list_open_todos": {"limit", "offset", "status"}, "get_runtime": {"hash", "with_payload"}, "resolve_runtime": {"hash", "note", "resolved_by"}, "get_todo": {"id", "with_network_payloads"}, "get_todo_image": {"id", "attachment_id"}, "update_todo_status": {"id", "status", "note", "by"}}
	keys, known := allowed[name]
	if !known {
		return toolError("Unknown tool"), nil
	}
	args, ok := readArgs(raw, keys)
	if !ok || ctx == nil {
		return toolError("Invalid tool arguments"), nil
	}
	switch name {
	case "list_open_runtimes", "list_open_todos":
		q, ok := listArgs(args)
		if !ok {
			return toolError("Invalid list arguments"), nil
		}
		var page cloudsync.MCPPage
		var err error
		key := "runtimes"
		if name == "list_open_runtimes" {
			page, err = t.client.FetchRuntimes(ctx, q)
		} else {
			key = "todos"
			page, err = t.client.FetchTodos(ctx, q)
		}
		if err != nil {
			return toolError("Cloud list failed"), nil
		}
		return structured(map[string]any{"count": len(page.Rows), "pagination_supported": page.PaginationSupported, "offset": page.Offset, "has_more": page.HasMore, "next_offset": page.NextOffset, key: page.Rows}, ""), nil
	case "get_runtime":
		hash, ok := requiredString(args, "hash")
		with, valid := booleanArg(args, "with_payload")
		if !ok || !valid {
			return toolError("Invalid runtime arguments"), nil
		}
		record, err := t.client.FetchRuntime(ctx, hash, with)
		if err != nil {
			return toolError("Cloud runtime read failed"), nil
		}
		text := stringValue(record["markdown"])
		delete(record, "markdown")
		if !with {
			delete(record, "payload")
		}
		return structured(map[string]any{"runtime": record}, text), nil
	case "resolve_runtime":
		hash, ok := requiredString(args, "hash")
		note, nok := optionalString(args, "note")
		by, bok := optionalString(args, "resolved_by")
		if !ok || !nok || !bok {
			return toolError("Invalid resolve arguments"), nil
		}
		body, err := t.client.ResolveRuntime(ctx, hash, note, by)
		if err != nil {
			return toolError("Cloud resolve failed"), nil
		}
		return statusResult(body, "runtime", "hash"), nil
	case "get_todo":
		id, ok := requiredString(args, "id")
		with, valid := booleanArg(args, "with_network_payloads")
		if !ok || !valid {
			return toolError("Invalid Todo arguments"), nil
		}
		todo, err := t.client.FetchTodo(ctx, id)
		if err != nil {
			return toolError("Cloud Todo read failed"), nil
		}
		text := stringValue(todo["markdown"])
		delete(todo, "markdown")
		if !with {
			if !compact(todo, "context_requests", []string{"id", "kind", "method", "url", "status", "ok", "duration", "startedAt", "error", "responseSizeBytes"}) {
				return toolError("Cloud Todo context invalid"), nil
			}
		}
		if !compact(todo, "context_errors", []string{"id", "level", "message", "stack", "source", "line", "col", "startedAt", "startTime"}) {
			return toolError("Cloud Todo context invalid"), nil
		}
		todo["network_payloads_included"], _ = json.Marshal(with)
		return structured(map[string]any{"todo": todo}, text), nil
	case "get_todo_image":
		id, ok := requiredString(args, "id")
		attachment, aok := requiredString(args, "attachment_id")
		if !ok || !aok {
			return toolError("Invalid image arguments"), nil
		}
		image, err := t.client.FetchTodoImage(ctx, id, attachment)
		if err != nil {
			return toolError("Cloud private image failed"), nil
		}
		result := structured(map[string]any{"attachment": image.Metadata}, "Private Todo image; use only for the authorized task.")
		result.Content = append(result.Content, mcp.Content{Type: "image", Data: image.Data, MimeType: image.MimeType})
		return result, nil
	case "update_todo_status":
		id, ok := requiredString(args, "id")
		status, sok := requiredString(args, "status")
		note, nok := optionalString(args, "note")
		by, bok := optionalString(args, "by")
		if !ok || !sok || !nok || !bok {
			return toolError("Invalid status arguments"), nil
		}
		body, err := t.client.UpdateTodoStatus(ctx, id, status, note, by)
		if err != nil {
			return toolError("Cloud status update failed"), nil
		}
		return statusResult(body, "todo", "id"), nil
	}
	return toolError("Unknown tool"), nil
}
func statusResult(body cloudsync.MCPObject, key, identity string) mcp.ToolResult {
	var record cloudsync.MCPObject
	if json.Unmarshal(body[key], &record) != nil {
		return toolError("Cloud status receipt invalid")
	}
	body[identity] = record[identity]
	body["status"] = record["status"]
	return structured(body, "")
}
func structured(value any, prefix string) mcp.ToolResult {
	data, err := json.Marshal(value)
	if err != nil {
		return toolError("Cloud output invalid")
	}
	text := string(data)
	if prefix != "" {
		text = prefix + "\n\n## Structured context\n" + text
	}
	return mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: text}}, StructuredContent: data}
}
func toolError(message string) mcp.ToolResult {
	return mcp.ToolResult{IsError: true, Content: []mcp.Content{{Type: "text", Text: message}}}
}
func readArgs(raw []byte, allowed []string) (map[string]json.RawMessage, bool) {
	if !utf8.Valid(raw) || len(raw) > 1<<20 {
		return nil, false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	out := map[string]json.RawMessage{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, false
		}
		name, ok := key.(string)
		if !ok {
			return nil, false
		}
		if _, duplicate := out[name]; duplicate {
			return nil, false
		}
		known := false
		for _, a := range allowed {
			if name == a {
				known = true
			}
		}
		if !known {
			return nil, false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, false
		}
		out[name] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, false
	}
	_, err = d.Token()
	return out, err == io.EOF
}
func requiredString(args map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := args[key]
	if !ok {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || !utf8.ValidString(value) {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}
func optionalString(args map[string]json.RawMessage, key string) (*string, bool) {
	raw, ok := args[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !utf8.ValidString(value) {
		return nil, false
	}
	return &value, true
}
func booleanArg(args map[string]json.RawMessage, key string) (bool, bool) {
	raw, ok := args[key]
	if !ok {
		return false, true
	}
	if bytes.Equal(raw, []byte("true")) {
		return true, true
	}
	if bytes.Equal(raw, []byte("false")) {
		return false, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil || bytes.Equal(raw, []byte("null")) {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "1", "true", "on", "yes":
		return true, true
	default:
		return false, true
	}
}
func listArgs(args map[string]json.RawMessage) (cloudsync.MCPListQuery, bool) {
	q := cloudsync.MCPListQuery{Limit: 20}
	for _, key := range []string{"limit", "offset"} {
		if raw, ok := args[key]; ok {
			n, err := strconv.ParseInt(string(raw), 10, 64)
			if err != nil {
				return q, false
			}
			if key == "limit" {
				if n < 1 || n > 50 {
					return q, false
				}
				q.Limit = int(n)
			} else {
				if n < 0 {
					return q, false
				}
				q.Offset = n
			}
		}
	}
	if _, ok := args["status"]; ok {
		value, valid := requiredString(args, "status")
		if !valid {
			return q, false
		}
		q.Status = value
	}
	return q, true
}
func compact(record cloudsync.MCPObject, key string, allowed []string) bool {
	raw, ok := record[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		record[key] = json.RawMessage("[]")
		return true
	}
	var rows []cloudsync.MCPObject
	if json.Unmarshal(raw, &rows) != nil {
		return false
	}
	for _, row := range rows {
		if row == nil {
			return false
		}
		for field := range row {
			keep := false
			for _, a := range allowed {
				if field == a {
					keep = true
				}
			}
			if !keep {
				delete(row, field)
			}
		}
	}
	record[key], _ = json.Marshal(rows)
	return true
}
func stringValue(raw []byte) string { var value string; json.Unmarshal(raw, &value); return value }
