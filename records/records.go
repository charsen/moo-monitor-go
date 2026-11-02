// Package records builds sanitized runtime snapshots without collecting or persisting them.
package records

import (
	"crypto/md5"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-go-monitor/mask"
)

type Config struct {
	MaskKeys         []string
	BasePath         string
	TraceMaxBytes    *int
	StringMaxChars   *int
	AppFramePrefixes []string
}

type Builder struct {
	masker                        *mask.Masker
	basePath                      string
	traceMaxBytes, stringMaxChars int
	prefixes                      []string
}

type Previous struct {
	Class   string `json:"class" yaml:"class"`
	Message string `json:"message" yaml:"message"`
	File    string `json:"file" yaml:"file"`
	Line    int    `json:"line" yaml:"line"`
}

type Exception struct {
	Class    string     `json:"class" yaml:"class"`
	Code     string     `json:"code" yaml:"code"`
	Message  string     `json:"message" yaml:"message"`
	File     string     `json:"file" yaml:"file"`
	Line     int        `json:"line" yaml:"line"`
	Previous []Previous `json:"previous,omitempty" yaml:"previous,omitempty"`
}

type Request struct {
	Method   string  `json:"method" yaml:"method"`
	URL      *string `json:"url" yaml:"url"`
	IP       *string `json:"ip" yaml:"ip"`
	UserID   *string `json:"user_id" yaml:"user_id"`
	UserName *string `json:"user_name" yaml:"user_name"`
	Guard    *string `json:"guard" yaml:"guard"`
}

type GoContext struct {
	// Legacy fields are only populated when importing real PHP records.
	PHP        *string `json:"php,omitempty" yaml:"php,omitempty"`
	Laravel    *string `json:"laravel,omitempty" yaml:"laravel,omitempty"`
	Env        string  `json:"env" yaml:"env"`
	Project    string  `json:"project" yaml:"project"`
	Go         string  `json:"go" yaml:"go"`
	Goravel    string  `json:"goravel" yaml:"goravel"`
	OccurredAt string  `json:"occurred_at" yaml:"occurred_at"`
}

type Frame struct {
	File     string `json:"file" yaml:"file"`
	Line     int    `json:"line" yaml:"line"`
	Function string `json:"function" yaml:"function"`
}

type Trace struct {
	Full   string  `json:"full" yaml:"full"`
	Frames []Frame `json:"app_frames" yaml:"app_frames"`
}

type Snippet struct {
	File     string `json:"file" yaml:"file"`
	Line     int    `json:"line" yaml:"line"`
	Language string `json:"language" yaml:"language"`
	Code     string `json:"code" yaml:"code"`
}

type Snapshot struct {
	Exception Exception
	Request   Request
	Context   GoContext
	Trace     Trace
	Snippet   Snippet
	Payload   any
	Source    string
	Meta      map[string]any
}

type Daily struct {
	Date  string `json:"date" yaml:"date"`
	Count int64  `json:"count" yaml:"count"`
}

type Record struct {
	Hash         string         `json:"hash" yaml:"hash"`
	FirstSeen    string         `json:"first_seen" yaml:"first_seen"`
	LastSeen     string         `json:"last_seen" yaml:"last_seen"`
	Count        int64          `json:"count" yaml:"count"`
	Daily        Daily          `json:"daily" yaml:"daily"`
	Status       string         `json:"status" yaml:"status"`
	ResolvedAt   *string        `json:"resolved_at" yaml:"resolved_at"`
	ResolvedBy   *string        `json:"resolved_by" yaml:"resolved_by"`
	ResolvedNote *string        `json:"resolved_note" yaml:"resolved_note"`
	Exception    Exception      `json:"exception" yaml:"exception"`
	Request      Request        `json:"request" yaml:"request"`
	Context      GoContext      `json:"context" yaml:"context"`
	Trace        Trace          `json:"trace" yaml:"trace"`
	Snippet      Snippet        `json:"source_snippet" yaml:"source_snippet"`
	Payload      any            `json:"payload" yaml:"payload"`
	Meta         map[string]any `json:"meta" yaml:"meta"`
}

func New(config Config) (*Builder, error) {
	masker, err := mask.New(config.MaskKeys)
	if err != nil {
		return nil, fmt.Errorf("records masker: %w", err)
	}
	builder := &Builder{masker: masker, basePath: config.BasePath, traceMaxBytes: 65536, stringMaxChars: 200, prefixes: []string{"app/", "routes/"}}
	if config.TraceMaxBytes != nil {
		builder.traceMaxBytes = *config.TraceMaxBytes
	}
	if config.StringMaxChars != nil {
		builder.stringMaxChars = *config.StringMaxChars
	}
	if config.AppFramePrefixes != nil {
		builder.prefixes = append([]string{}, config.AppFramePrefixes...)
	}
	if !utf8.ValidString(config.BasePath) {
		return nil, fmt.Errorf("invalid record path or trace limit")
	}
	for _, prefix := range builder.prefixes {
		if !utf8.ValidString(prefix) {
			return nil, fmt.Errorf("invalid frame prefix UTF-8")
		}
	}
	return builder, nil
}

// Build consumes one explicit timestamp for the entire record and context.
func (b *Builder) Build(snapshot Snapshot, now time.Time) (*Record, error) {
	latest, err := b.prepare(snapshot, now, nil)
	if err != nil {
		return nil, err
	}
	latest.Hash = b.Hash(snapshot.Exception)
	latest.FirstSeen = latest.LastSeen
	latest.Count = 1
	latest.Daily = Daily{Date: latest.LastSeen[:10], Count: 1}
	latest.Status = "open"
	return latest, nil
}

// Refresh preserves identity, first_seen and resolution state. Bucket reopen is a later layer.
func (b *Builder) Refresh(existing Record, snapshot Snapshot, now time.Time, overflow int64) (*Record, error) {
	extra := max(int64(0), overflow)
	if existing.Count < 0 || existing.Daily.Count < 0 || extra == math.MaxInt64 || existing.Count > math.MaxInt64-1-extra {
		return nil, fmt.Errorf("runtime count overflow")
	}
	if existing.Daily.Date == now.Format("2006-01-02") && existing.Daily.Count == math.MaxInt64 {
		return nil, fmt.Errorf("daily count overflow")
	}
	latest, err := b.prepare(snapshot, now, existing.Meta)
	if err != nil {
		return nil, err
	}
	latest.Hash, latest.FirstSeen = existing.Hash, existing.FirstSeen
	latest.Count = existing.Count + 1 + extra
	latest.Daily = Daily{Date: latest.LastSeen[:10], Count: 1}
	if existing.Daily.Date == latest.Daily.Date {
		latest.Daily.Count = existing.Daily.Count + 1
	}
	latest.Status = existing.Status
	latest.ResolvedAt, latest.ResolvedBy, latest.ResolvedNote = copyString(existing.ResolvedAt), copyString(existing.ResolvedBy), copyString(existing.ResolvedNote)
	return latest, nil
}

func (b *Builder) prepare(s Snapshot, now time.Time, existingMeta map[string]any) (*Record, error) {
	if now.Year() < 0 || now.Year() > 9999 {
		return nil, fmt.Errorf("record timestamp year outside ISO range")
	}
	// Payload's strict representation validation happens before a record is returned.
	var payload any = []any{}
	switch strings.ToUpper(s.Request.Method) {
	case "POST", "PUT", "PATCH", "DELETE":
		if s.Payload != nil {
			var err error
			payload, err = b.masker.MaskPayload(s.Payload, b.stringMaxChars)
			if err != nil {
				return nil, fmt.Errorf("runtime payload: %w", err)
			}
		}
	}
	meta, err := b.runtimeMeta(s.Source, s.Meta, existingMeta)
	if err != nil {
		return nil, err
	}
	exception := s.Exception
	exception.Message = b.doubleMask(exception.Message)
	exception.File = b.relPath(exception.File)
	exception.Previous = make([]Previous, 0, min(len(s.Exception.Previous), 3))
	for _, previous := range s.Exception.Previous[:min(len(s.Exception.Previous), 3)] {
		previous.Message = b.doubleMask(previous.Message)
		previous.File = b.relPath(previous.File)
		exception.Previous = append(exception.Previous, previous)
	}
	request := s.Request
	request.URL, request.IP, request.UserID, request.UserName, request.Guard = copyString(request.URL), copyString(request.IP), copyString(request.UserID), copyString(request.UserName), copyString(request.Guard)
	if request.URL != nil {
		*request.URL = b.masker.MaskURL(*request.URL)
	}
	full := b.doubleMask(s.Trace.Full)
	if len(full) > b.traceMaxBytes {
		end := b.traceMaxBytes
		if end < 0 {
			end = max(0, len(full)+end)
		}
		full = full[:end] + "\n... (truncated)"
	}
	trace := Trace{Full: full, Frames: []Frame{}}
	for _, frame := range s.Trace.Frames {
		frame.File = b.relPath(frame.File)
		matched := false
		for _, prefix := range b.prefixes {
			if prefix != "" && strings.HasPrefix(frame.File, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		frame.Function = strings.Trim(frame.Function, ":")
		trace.Frames = append(trace.Frames, frame)
		if len(trace.Frames) == 20 {
			break
		}
	}
	snippet := s.Snippet
	snippet.File = b.relPath(snippet.File) // Code stays source text, exactly as in PHP. No file is read.
	context := s.Context
	context.PHP, context.Laravel = copyString(context.PHP), copyString(context.Laravel)
	for _, value := range []*string{context.PHP, context.Laravel} {
		if value != nil && !utf8.ValidString(*value) {
			return nil, fmt.Errorf("invalid legacy context UTF-8")
		}
	}
	timestamp := now.Format("2006-01-02T15:04:05.000Z07:00")
	// PHP emits +00:00, never Z.
	if strings.HasSuffix(timestamp, "Z") {
		timestamp = strings.TrimSuffix(timestamp, "Z") + "+00:00"
	}
	context.OccurredAt = timestamp
	return &Record{LastSeen: timestamp, Exception: exception, Request: request, Context: context, Trace: trace, Snippet: snippet, Payload: payload, Meta: meta}, nil
}

func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func (b *Builder) doubleMask(value string) string {
	return b.masker.MaskSecrets(b.masker.MaskSensitiveSQL(value))
}
func (b *Builder) relPath(path string) string {
	base := strings.TrimRight(b.basePath, "/")
	if b.basePath != "" && strings.HasPrefix(path, base) {
		return strings.TrimLeft(strings.TrimPrefix(path, base), "/")
	}
	return path
}

var normalization = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`'[^']*'`), "'X'"},
	{regexp.MustCompile(`"[^"]*"`), `"X"`},
	{regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`), "U"},
	{regexp.MustCompile(`(?i)\b0x[0-9a-f]+\b`), "0xH"},
	{regexp.MustCompile(`(?i)\b[0-9a-f]{16,}\b`), "H"},
	{regexp.MustCompile(`[0-9]+`), "N"},
}

func (b *Builder) normalizeMessage(message string) string {
	message = b.doubleMask(message)
	for _, rule := range normalization {
		message = rule.pattern.ReplaceAllString(message, rule.replacement)
	}
	if len(message) > 1024 {
		message = message[:1024]
	}
	return message
}

// Hash is byte based; JSON, context, code and timestamps do not participate.
// Inputs must carry a stable explicit class/file/line identity from the collector.
func (b *Builder) Hash(exception Exception) string {
	identity := fmt.Sprintf("%s|%s:%d|%s", exception.Class, b.relPath(exception.File), exception.Line, b.normalizeMessage(exception.Message))
	return fmt.Sprintf("%x", md5.Sum([]byte(identity)))[:12]
}

// SourceRank is the single priority authority shared with future dispatchers.
func SourceRank(source string) int {
	switch source {
	case "queue_failed":
		return 30
	case "schedule_exit":
		return 28
	case "http_5xx":
		return 25
	case "log_context":
		return 20
	case "log_message":
		return 15
	case "reportable":
		return 10
	}
	return 0
}

func NormalizeSource(source string) string {
	var normalized strings.Builder
	for _, char := range []byte(source) {
		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			normalized.WriteByte(char)
		} else {
			normalized.WriteByte('_')
		}
	}
	// PHP's ?: treats the literal "0" as false before trimming.
	raw := normalized.String()
	if raw == "0" {
		raw = "reportable"
	}
	value := strings.Trim(raw, "_-")
	if value == "" {
		return "reportable"
	}
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func (b *Builder) runtimeMeta(source string, incoming, existing map[string]any) (map[string]any, error) {
	source = NormalizeSource(source)
	current, _ := existing["source"].(string)
	sources := []string{}
	addSource := func(value string) {
		if value == "" || value == "0" {
			return
		}
		for _, old := range sources {
			if value == old {
				return
			}
		}
		sources = append(sources, value)
	}
	switch old := existing["sources"].(type) {
	case []string:
		for _, value := range old {
			addSource(value)
		}
	case []any:
		for _, value := range old {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("legacy sources must be strings")
			}
			addSource(text)
		}
	case string:
		addSource(old)
	case nil:
	default:
		return nil, fmt.Errorf("unsupported legacy sources")
	}
	addSource(current)
	addSource(source)
	merged := make(map[string]any, len(existing)+len(incoming))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range incoming {
		merged[key] = value
	}
	clean := make(map[string]any, len(merged))
	for key, value := range merged {
		if key == "source" || key == "sources" {
			continue
		}
		switch scalar := value.(type) {
		case nil:
			if b.masker.ShouldMaskKey(key) {
				clean[key] = "***"
			}
		case string:
			if !utf8.ValidString(scalar) {
				return nil, fmt.Errorf("metadata string must be valid UTF-8")
			}
			if b.masker.ShouldMaskKey(key) {
				clean[key] = "***"
				continue
			}
			scalar = b.doubleMask(scalar)
			// Cut by character after redaction; no full rune copy needed.
			count := 0
			for index := range scalar {
				if count == 500 {
					scalar = scalar[:index]
					break
				}
				count++
			}
			if scalar != "" {
				clean[key] = scalar
			}
		case int:
			if b.masker.ShouldMaskKey(key) {
				clean[key] = "***"
			} else {
				clean[key] = int64(scalar)
			}
		case bool, int64:
			if b.masker.ShouldMaskKey(key) {
				clean[key] = "***"
			} else {
				clean[key] = scalar
			}
		case float64:
			if math.IsNaN(scalar) || math.IsInf(scalar, 0) {
				return nil, fmt.Errorf("metadata float must be finite")
			}
			if b.masker.ShouldMaskKey(key) {
				clean[key] = "***"
			} else {
				clean[key] = scalar
			}
			// Nested metadata is ignored by the source. It does not become payload.
		}
	}
	if current != "" && SourceRank(source) < SourceRank(current) {
		clean["source"] = current
	} else {
		clean["source"] = source
	}
	clean["sources"] = sources
	return clean, nil
}

// TagMetadata reuses runtime sanitization without refreshing occurrence fields or counters.
func (b *Builder) TagMetadata(existing Record, source string, meta map[string]any) (*Record, bool, error) {
	clean, err := b.runtimeMeta(source, meta, existing.Meta)
	if err != nil {
		return nil, false, err
	}
	if sameMetadata(existing.Meta, clean) {
		return &existing, false, nil
	}
	existing.Meta = clean
	return &existing, true, nil
}

func sameMetadata(left, right map[string]any) bool {
	count := 0
	for key := range left {
		if key != "updated_at" {
			count++
		}
	}
	other := 0
	for key := range right {
		if key != "updated_at" {
			other++
		}
	}
	if count != other {
		return false
	}
	for key, want := range right {
		if key == "updated_at" {
			continue
		}
		got, exists := left[key]
		if !exists {
			return false
		}
		switch value := want.(type) {
		case string:
			if actual, ok := got.(string); !ok || actual != value {
				return false
			}
		case bool:
			if actual, ok := got.(bool); !ok || actual != value {
				return false
			}
		case int64:
			switch actual := got.(type) {
			case int:
				if int64(actual) != value {
					return false
				}
			case int64:
				if actual != value {
					return false
				}
			default:
				return false
			}
		case float64:
			if actual, ok := got.(float64); !ok || actual != value {
				return false
			}
		case nil:
			if got != nil {
				return false
			}
		case []string:
			switch actual := got.(type) {
			case []string:
				if len(actual) != len(value) {
					return false
				}
				for i := range value {
					if actual[i] != value[i] {
						return false
					}
				}
			case []any:
				if len(actual) != len(value) {
					return false
				}
				for i := range value {
					item, ok := actual[i].(string)
					if !ok || item != value[i] {
						return false
					}
				}
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}
