package cloudsync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"
)

type MCPObject map[string]json.RawMessage
type MCPListQuery struct {
	Limit  int
	Offset int64
	Status string
}
type MCPPage struct {
	Rows                []MCPObject
	Offset              int64
	PaginationSupported bool
	HasMore             *bool
	NextOffset          *int64
}
type MCPImage struct {
	Metadata       MCPObject
	Data, MimeType string
}

var mcpHash = regexp.MustCompile(`^[a-f0-9]{12}$`)
var mcpTodo = regexp.MustCompile(`^[a-zA-Z0-9]{20,32}$`)
var mcpAttachment = regexp.MustCompile(`^[0-9]{1,32}$`)
var ErrMCPInput = errors.New("Cloud MCP input rejected")

func (c *HTTPClient) FetchRuntimes(ctx context.Context, q MCPListQuery) (MCPPage, error) {
	return c.mcpList(ctx, "api/v1/runtimes/list", "runtimes", q, []string{"open", "in_progress", "resolved"})
}
func (c *HTTPClient) FetchTodos(ctx context.Context, q MCPListQuery) (MCPPage, error) {
	return c.mcpList(ctx, "api/v1/todos/list", "todos", q, []string{"open", "in_progress", "done"})
}
func (c *HTTPClient) FetchRuntime(ctx context.Context, hash string, withPayload bool) (MCPObject, error) {
	if !mcpHash.MatchString(hash) {
		return nil, ErrMCPInput
	}
	body, err := c.mcpRead(ctx, "api/v1/runtimes/get", map[string]any{"hash": hash, "with_payload": withPayload})
	if err != nil {
		return nil, err
	}
	record, err := mcpDetail(body, "runtime", "hash", hash)
	if !withPayload {
		delete(record, "payload")
	}
	return record, err
}
func (c *HTTPClient) FetchTodo(ctx context.Context, id string) (MCPObject, error) {
	if !mcpTodo.MatchString(id) {
		return nil, ErrMCPInput
	}
	body, err := c.mcpRead(ctx, "api/v1/todos/get", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	return mcpDetail(body, "todo", "id", id)
}
func (c *HTTPClient) ResolveRuntime(ctx context.Context, hash string, note, by *string) (MCPObject, error) {
	if !mcpHash.MatchString(hash) {
		return nil, ErrMCPInput
	}
	fields := map[string]any{"hash": hash}
	if !mcpOptional(fields, "note", note) || !mcpOptional(fields, "resolved_by", by) {
		return nil, ErrMCPInput
	}
	body, err := c.mcpRead(ctx, "api/v1/runtimes/resolve", fields)
	if err != nil {
		return nil, err
	}
	record, err := mcpDetail(body, "runtime", "hash", hash)
	if err != nil || rawString(record["status"]) != "resolved" {
		return nil, ErrHTTPReceipt
	}
	if !mcpFlag(body, "already_resolved") {
		return nil, ErrHTTPReceipt
	}
	return MCPObject{"runtime": body["runtime"], "already_resolved": mcpFlagValue(body, "already_resolved")}, nil
}
func (c *HTTPClient) UpdateTodoStatus(ctx context.Context, id, status string, note, by *string) (MCPObject, error) {
	if !mcpTodo.MatchString(id) || (status != "done" && status != "in_progress") {
		return nil, ErrMCPInput
	}
	fields := map[string]any{"id": id, "status": status}
	if !mcpOptional(fields, "note", note) || !mcpOptional(fields, "by", by) {
		return nil, ErrMCPInput
	}
	body, err := c.mcpRead(ctx, "api/v1/todos/status", fields)
	if err != nil {
		return nil, err
	}
	record, err := mcpDetail(body, "todo", "id", id)
	if err != nil || (rawString(record["status"]) != "done" && rawString(record["status"]) != "in_progress") {
		return nil, ErrHTTPReceipt
	}
	if !mcpFlag(body, "already_done") || !mcpFlag(body, "already_in_progress") {
		return nil, ErrHTTPReceipt
	}
	alreadyDone := bytes.Equal(bytes.TrimSpace(body["already_done"]), []byte("true"))
	alreadyProgress := bytes.Equal(bytes.TrimSpace(body["already_in_progress"]), []byte("true"))
	actual := rawString(record["status"])
	if (alreadyDone && alreadyProgress) || (alreadyDone && actual != "done") || (alreadyProgress && (status != "in_progress" || actual != "in_progress")) || (actual != status && !alreadyDone) {
		return nil, ErrHTTPReceipt
	}
	return MCPObject{"todo": body["todo"], "already_done": mcpFlagValue(body, "already_done"), "already_in_progress": mcpFlagValue(body, "already_in_progress")}, nil
}
func (c *HTTPClient) FetchTodoImage(ctx context.Context, id, attachment string) (MCPImage, error) {
	if !mcpTodo.MatchString(id) || !mcpAttachment.MatchString(attachment) {
		return MCPImage{}, ErrMCPInput
	}
	body, err := c.mcpRead(ctx, "api/v1/todos/image", map[string]any{"id": id, "attachment_id": attachment})
	if err != nil {
		return MCPImage{}, err
	}
	image, err := mcpDetail(body, "image", "todo_id", id)
	if err != nil {
		return MCPImage{}, err
	}
	data, mime := rawString(image["data"]), rawString(image["mime_type"])
	if len(data) > base64.StdEncoding.EncodedLen(2<<20) {
		return MCPImage{}, ErrHTTPReceipt
	}
	binary, err := base64.StdEncoding.Strict().DecodeString(data)
	if err != nil || len(data) > base64.StdEncoding.EncodedLen(2<<20) || len(binary) > 2<<20 || data == "" || base64.StdEncoding.EncodeToString(binary) != data {
		return MCPImage{}, ErrHTTPReceipt
	}
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
	default:
		return MCPImage{}, ErrHTTPReceipt
	}
	imageID := rawString(image["id"])
	if imageID == "" {
		var n json.Number
		if json.Unmarshal(image["id"], &n) == nil {
			imageID = n.String()
		}
	}
	if imageID != attachment {
		return MCPImage{}, ErrHTTPReceipt
	}
	delete(image, "data")
	return MCPImage{image, data, mime}, nil
}
func (c *HTTPClient) mcpList(ctx context.Context, path, key string, q MCPListQuery, statuses []string) (MCPPage, error) {
	if q.Limit < 1 || q.Limit > 50 || q.Offset < 0 {
		return MCPPage{}, ErrMCPInput
	}
	if q.Status != "" {
		found := false
		for _, s := range statuses {
			if s == q.Status {
				found = true
			}
		}
		if !found {
			return MCPPage{}, ErrMCPInput
		}
	}
	fields := map[string]any{"limit": q.Limit, "offset": q.Offset}
	if q.Status != "" {
		fields["status"] = q.Status
	}
	body, err := c.mcpRead(ctx, path, fields)
	if err != nil {
		return MCPPage{}, err
	}
	var rows []MCPObject
	if len(body[key]) == 0 || bytes.Equal(body[key], []byte("null")) || json.Unmarshal(body[key], &rows) != nil || len(rows) > q.Limit {
		return MCPPage{}, ErrHTTPReceipt
	}
	for _, row := range rows {
		if len(row) == 0 {
			return MCPPage{}, ErrHTTPReceipt
		}
	}
	if raw, ok := body["count"]; ok {
		n, valid := mcpInteger(raw)
		if !valid || n != int64(len(rows)) {
			return MCPPage{}, ErrHTTPReceipt
		}
	}
	page := MCPPage{Rows: rows, Offset: q.Offset}
	present := 0
	for _, key := range []string{"offset", "has_more", "next_offset"} {
		if _, ok := body[key]; ok {
			present++
		}
	}
	if present == 0 {
		if q.Offset != 0 {
			return MCPPage{}, ErrHTTPReceipt
		}
		return page, nil
	}
	if present != 3 {
		return MCPPage{}, ErrHTTPReceipt
	}
	offset, ok := mcpInteger(body["offset"])
	if !ok || offset != q.Offset {
		return MCPPage{}, ErrHTTPReceipt
	}
	var more bool
	if !mcpBool(body["has_more"], &more) {
		return MCPPage{}, ErrHTTPReceipt
	}
	page.PaginationSupported = true
	page.HasMore = &more
	if more {
		n, ok := mcpInteger(body["next_offset"])
		if !ok || n <= offset {
			return MCPPage{}, ErrHTTPReceipt
		}
		page.NextOffset = &n
	} else if !bytes.Equal(bytes.TrimSpace(body["next_offset"]), []byte("null")) {
		return MCPPage{}, ErrHTTPReceipt
	}
	return page, nil
}
func (c *HTTPClient) mcpRead(ctx context.Context, path string, fields map[string]any) (MCPObject, error) {
	if c == nil || c.client == nil {
		return nil, ErrHTTPConfig
	}
	fields["token"] = c.token
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, ErrWire
	}
	operation := *c
	client := *c.client
	client.Timeout = max(time.Second, min(client.Timeout, 4*time.Second))
	operation.client = &client
	data, status, attempt, err := operation.post(ctx, path, data, 2, 100*time.Millisecond)
	if err != nil {
		var failure *HTTPError
		if errors.As(err, &failure) {
			copy := *failure
			copy.Operation = "Cloud MCP"
			return nil, &copy
		}
		return nil, ErrHTTPTransport
	}
	var result MCPObject
	if !utf8.Valid(data) || !mcpUnique(data) || decodeJSON(data, &result) != nil || result == nil || !bytes.Equal(bytes.TrimSpace(result["ok"]), []byte("true")) {
		return nil, &HTTPError{Operation: "Cloud MCP", Status: status, Attempt: attempt, category: ErrHTTPReceipt}
	}
	return result, nil
}
func mcpDetail(body MCPObject, key, identity, value string) (MCPObject, error) {
	var result MCPObject
	if json.Unmarshal(body[key], &result) != nil || len(result) == 0 || rawString(result[identity]) != value {
		return nil, ErrHTTPReceipt
	}
	return result, nil
}
func rawString(b []byte) string {
	var v string
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	return v
}
func mcpOptional(fields map[string]any, key string, value *string) bool {
	if value != nil {
		if !utf8.ValidString(*value) {
			return false
		}
		fields[key] = *value
	}
	return true
}
func mcpFlag(body MCPObject, key string) bool {
	raw, ok := body[key]
	if !ok {
		return true
	}
	var b bool
	return mcpBool(raw, &b)
}
func mcpBool(raw []byte, b *bool) bool {
	raw = bytes.TrimSpace(raw)
	if !bytes.Equal(raw, []byte("true")) && !bytes.Equal(raw, []byte("false")) {
		return false
	}
	return json.Unmarshal(raw, b) == nil
}
func mcpInteger(raw []byte) (int64, bool) {
	n, err := strconv.ParseInt(string(raw), 10, 64)
	return n, err == nil
}

// MCP responses carry nested Cloud context, unlike the shallow private ledger schema.
func mcpUnique(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var walk func(int) bool
	walk = func(depth int) bool {
		if depth > 64 {
			return false
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		v, ok := token.(json.Delim)
		if !ok {
			return true
		}
		switch v {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return false
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return false
				}
				seen[s] = true
				if !walk(depth + 1) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for d.More() {
				if !walk(depth + 1) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim(']')
		}
		return false
	}
	return walk(0)
}

func mcpFlagValue(body MCPObject, key string) json.RawMessage {
	if bytes.Equal(bytes.TrimSpace(body[key]), []byte("true")) {
		return json.RawMessage("true")
	}
	return json.RawMessage("false")
}
