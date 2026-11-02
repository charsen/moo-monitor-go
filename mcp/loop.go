// Package mcp provides the fixed-version stdio transport, independently of Cloud tools.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

var ErrConfig = errors.New("MCP configuration rejected")
var ErrInput = errors.New("MCP input limit or transport failure")
var ErrOutput = errors.New("MCP output failure")

const newest = "2025-06-18"

type Config struct {
	ServerName, Version           string
	MaxInputBytes, MaxOutputBytes int
}
type ToolDefinition struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	Annotations  *Annotations    `json:"annotations,omitempty"`
}
type Annotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// MarshalJSON preserves the required text field even for an empty text block.
func (c Content) MarshalJSON() ([]byte, error) {
	switch c.Type {
	case "text":
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{c.Type, c.Text})
	case "image":
		return json.Marshal(struct {
			Type     string `json:"type"`
			Data     string `json:"data"`
			MimeType string `json:"mimeType"`
		}{c.Type, c.Data, c.MimeType})
	default:
		return nil, ErrOutput
	}
}

type ToolResult struct {
	Content           []Content       `json:"content"`
	IsError           bool            `json:"isError,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
}
type Toolset interface {
	Definitions(context.Context) ([]ToolDefinition, error)
	Call(context.Context, string, json.RawMessage) (ToolResult, error)
	Instructions() string
}
type Server struct {
	tools Toolset
	cfg   Config
}

func NewServer(tools Toolset, cfg Config) (*Server, error) {
	if tools == nil || cfg.ServerName == "" || cfg.Version == "" || !utf8.ValidString(cfg.ServerName) || !utf8.ValidString(cfg.Version) {
		return nil, ErrConfig
	}
	if cfg.MaxInputBytes == 0 {
		cfg.MaxInputBytes = 1 << 20
	}
	if cfg.MaxOutputBytes == 0 {
		cfg.MaxOutputBytes = 4 << 20
	}
	if cfg.MaxInputBytes < 256 || cfg.MaxOutputBytes < 256 || cfg.MaxInputBytes > 64<<20 || cfg.MaxOutputBytes > 64<<20 {
		return nil, ErrConfig
	}
	return &Server{tools, cfg}, nil
}

// Serve owns input. Its Close must promptly unblock Read; arbitrary writers/tools must
// independently cooperate with context. A Server may serve separate sessions concurrently.
func (s *Server) Serve(ctx context.Context, input io.ReadCloser, stdout, stderr io.Writer) error {
	if ctx == nil || input == nil || stdout == nil || stderr == nil {
		return ErrConfig
	}
	if ctx.Err() != nil {
		input.Close()
		return ctx.Err()
	}
	stop := make(chan struct{})
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			input.Close()
		case <-stop:
		}
	}()
	defer func() { close(stop); input.Close(); <-joined }()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, min(4096, s.cfg.MaxInputBytes)), s.cfg.MaxInputBytes+1)
	session := session{server: s, version: newest}
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Bytes()
		if len(line) > s.cfg.MaxInputBytes {
			return ErrInput
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		before := session
		reply, diagnostic := session.dispatch(ctx, line)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if diagnostic {
			if n, err := io.WriteString(stderr, "[moo:mcp] request failed\n"); err != nil || n != len("[moo:mcp] request failed\n") {
				return ErrOutput
			}
		}
		if reply == nil {
			continue
		}
		encoded, err := json.Marshal(reply)
		if err != nil || len(encoded) > s.cfg.MaxOutputBytes {
			session = before
			encoded, _ = json.Marshal(failure(reply.ID, -32603, "Internal error"))
		}
		if len(encoded) > s.cfg.MaxOutputBytes {
			return ErrOutput
		}
		encoded = append(encoded, '\n')
		n, err := stdout.Write(encoded)
		if err != nil || n != len(encoded) {
			return ErrOutput
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if scanner.Err() != nil {
		return ErrInput
	}
	return nil
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func success(id json.RawMessage, result any) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}
func failure(id json.RawMessage, code int, message string) *response {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{code, message}}
}

type session struct {
	server             *Server
	version            string
	initialized, ready bool
}

func (s *session) dispatch(ctx context.Context, line []byte) (out *response, diagnostic bool) {
	var id json.RawMessage
	defer func() {
		if recover() != nil {
			out = failure(id, -32603, "Internal error")
			diagnostic = true
		}
	}()
	if !utf8.Valid(line) || !json.Valid(line) {
		return failure(nil, -32700, "Parse error"), false
	}
	if !strictJSON(line) {
		return failure(nil, -32600, "Invalid Request"), false
	}
	var msg map[string]json.RawMessage
	if json.Unmarshal(line, &msg) != nil || msg == nil || bytes.TrimSpace(line)[0] != '{' {
		return failure(nil, -32600, "Invalid Request"), false
	}
	id, hasID := msg["id"]
	if hasID && !validID(id) {
		return failure(nil, -32600, "Invalid Request"), false
	}
	var version, method string
	if json.Unmarshal(msg["jsonrpc"], &version) != nil || version != "2.0" || json.Unmarshal(msg["method"], &method) != nil || method == "" {
		return failure(id, -32600, "Invalid Request"), false
	}
	params := map[string]json.RawMessage{}
	if raw, ok := msg["params"]; ok {
		if !object(raw) || json.Unmarshal(raw, &params) != nil {
			return failure(id, -32602, "Invalid params"), false
		}
	}
	if !hasID {
		if method == "notifications/initialized" && s.initialized {
			s.ready = true
		}
		return nil, false
	}
	switch method {
	case "initialize":
		if s.initialized {
			return failure(id, -32600, "Invalid Request"), false
		}
		requested := ""
		if raw, ok := params["protocolVersion"]; ok {
			if json.Unmarshal(raw, &requested) != nil || bytes.Equal(raw, []byte("null")) {
				return failure(id, -32602, "Invalid params"), false
			}
		}
		version := newest
		for _, v := range []string{newest, "2025-03-26", "2024-11-05"} {
			if requested == v {
				version = v
			}
		}
		instructions := s.server.tools.Instructions()
		if !utf8.ValidString(instructions) {
			return failure(id, -32603, "Internal error"), true
		}
		s.version = version
		s.initialized = true
		return success(id, map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]bool{"listChanged": false}}, "serverInfo": map[string]string{"name": s.server.cfg.ServerName, "version": s.server.cfg.Version}, "instructions": instructions}), false
	case "ping":
		return success(id, struct{}{}), false
	case "tools/list":
		if !s.ready {
			return failure(id, -32600, "Invalid Request"), false
		}
		definitions, err := s.server.tools.Definitions(ctx)
		if err != nil {
			return failure(id, -32603, "Internal error"), true
		}
		definitions = append([]ToolDefinition(nil), definitions...)
		if definitions == nil {
			definitions = []ToolDefinition{}
		}
		for i := range definitions {
			d := &definitions[i]
			if !validDefinition(*d) {
				return failure(id, -32603, "Internal error"), true
			}
			if s.version != newest {
				d.OutputSchema = nil
			}
		}
		return success(id, map[string]any{"tools": definitions}), false
	case "tools/call":
		if !s.ready {
			return failure(id, -32600, "Invalid Request"), false
		}
		var name string
		if json.Unmarshal(params["name"], &name) != nil || name == "" {
			return failure(id, -32602, "Invalid params"), false
		}
		args := json.RawMessage("{}")
		if raw, ok := params["arguments"]; ok {
			if !object(raw) {
				return failure(id, -32602, "Invalid params"), false
			}
			args = raw
		}
		result, err := s.server.tools.Call(ctx, name, args)
		if err != nil || !validResult(result) {
			return failure(id, -32603, "Internal error"), true
		}
		if len(result.StructuredContent) > 0 {
			hasText := false
			for _, block := range result.Content {
				if block.Type == "text" {
					hasText = true
					break
				}
			}
			if !hasText {
				result.Content = append(append([]Content(nil), result.Content...), Content{Type: "text", Text: string(result.StructuredContent)})
			}
		}
		if result.Content == nil {
			result.Content = []Content{}
		}
		if s.version != newest {
			result.StructuredContent = nil
		}
		return success(id, result), false
	default:
		return failure(id, -32601, "Method not found"), false
	}
}
func validID(id json.RawMessage) bool {
	b := bytes.TrimSpace(id)
	return bytes.Equal(b, []byte("null")) || (len(b) > 0 && (b[0] == '-' || (b[0] >= '0' && b[0] <= '9'))) || (len(b) > 0 && b[0] == '"')
}
func object(b []byte) bool { b = bytes.TrimSpace(b); return len(b) > 0 && b[0] == '{' && strictJSON(b) }
func validDefinition(d ToolDefinition) bool {
	return d.Name != "" && utf8.ValidString(d.Name) && utf8.ValidString(d.Description) && object(d.InputSchema) && (len(d.OutputSchema) == 0 || object(d.OutputSchema)) && (d.Annotations == nil || utf8.ValidString(d.Annotations.Title))
}
func validResult(r ToolResult) bool {
	if len(r.StructuredContent) > 0 && !object(r.StructuredContent) {
		return false
	}
	for _, c := range r.Content {
		if !utf8.ValidString(c.Text) || !utf8.ValidString(c.Data) || !utf8.ValidString(c.MimeType) {
			return false
		}
		switch c.Type {
		case "text":
			if c.Data != "" || c.MimeType != "" {
				return false
			}
		case "image":
			if c.Data == "" || c.MimeType == "" || c.Text != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// strictJSON rejects duplicate object keys and depth bombs without coercing numbers.
func strictJSON(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if !jsonValue(d, 0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}
func jsonValue(d *json.Decoder, depth int) bool {
	if depth > 64 {
		return false
	}
	t, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return false
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return false
			}
			seen[key] = true
			if !jsonValue(d, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for d.More() {
			if !jsonValue(d, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}
