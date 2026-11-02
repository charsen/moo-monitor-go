package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"
	"unicode/utf8"
)

type SummaryProject struct {
	Name string  `json:"name"`
	Slug string  `json:"slug"`
	Env  *string `json:"env"`
	URL  *string `json:"url"`
	Icon *string `json:"icon"`
}
type SummaryCounts struct {
	Open  int64 `json:"open"`
	Total int64 `json:"total"`
}
type SummaryTodoCounts struct {
	Open       int64 `json:"open"`
	InProgress int64 `json:"in_progress"`
	Done       int64 `json:"done"`
	Total      int64 `json:"total"`
}
type SummaryStats struct {
	Runtimes    SummaryCounts     `json:"runtimes"`
	SlowQueries SummaryCounts     `json:"slow_queries"`
	Todos       SummaryTodoCounts `json:"todos"`
}
type SummaryRuntime struct {
	Hash             string  `json:"hash"`
	ExceptionClass   *string `json:"exc_class"`
	ExceptionMessage *string `json:"exc_message"`
	Count            int64   `json:"count"`
	LastSeen         *string `json:"last_seen"`
	Status           string  `json:"status"`
}
type SummarySlowQuery struct {
	Hash       string  `json:"hash"`
	SQLPreview *string `json:"sql_preview"`
	MaxMillis  float64 `json:"max_ms"`
	Count      int64   `json:"count"`
	LastSeen   *string `json:"last_seen"`
	Status     string  `json:"status"`
	File       *string `json:"at_file"`
	Line       *int64  `json:"at_line"`
}
type SummaryTodo struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
	Priority  string  `json:"priority"`
	CreatedAt *string `json:"created_at"`
}
type SummaryRecent struct {
	Runtimes    []SummaryRuntime   `json:"runtimes"`
	SlowQueries []SummarySlowQuery `json:"slow_queries"`
	Todos       []SummaryTodo      `json:"todos"`
}
type Summary struct {
	Project     SummaryProject `json:"project"`
	Stats       SummaryStats   `json:"stats"`
	Recent      SummaryRecent  `json:"recent"`
	GeneratedAt string         `json:"generated_at"`
}

// Summary is an explicit read operation; it neither pushes nor heartbeats.
func (c *HTTPClient) Summary(ctx context.Context, limit int) (Summary, error) {
	if limit < 1 || limit > 10 {
		return Summary{}, ErrWire
	}
	if c == nil || c.client == nil {
		return Summary{}, ErrHTTPConfig
	}
	body, err := json.Marshal(struct {
		Token string `json:"token"`
		Limit int    `json:"limit"`
	}{c.token, limit})
	if err != nil {
		return Summary{}, ErrWire
	}
	operation := *c
	client := *c.client
	client.Timeout = max(time.Second, min(client.Timeout, 4*time.Second))
	operation.client = &client
	data, status, attempt, err := operation.post(ctx, "api/v1/summary", body, 2, 100*time.Millisecond)
	if err != nil {
		var failure *HTTPError
		if errors.As(err, &failure) {
			copy := *failure
			copy.Operation = "summary"
			return Summary{}, &copy
		}
		return Summary{}, &HTTPError{Operation: "summary", category: ErrHTTPTransport}
	}
	result, err := decodeSummary(data, limit)
	if err != nil {
		return Summary{}, &HTTPError{Operation: "summary", Status: status, Attempt: attempt, category: ErrHTTPReceipt}
	}
	return result, nil
}
func summaryObject(raw json.RawMessage, required, nullable []string) (MCPObject, bool) {
	var object MCPObject
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, false
	}
	for _, key := range required {
		if len(object[key]) == 0 || bytes.Equal(bytes.TrimSpace(object[key]), []byte("null")) {
			return nil, false
		}
	}
	for _, key := range nullable {
		if len(object[key]) == 0 {
			return nil, false
		}
	}
	return object, true
}
func summaryTime(value *string) bool {
	if value == nil {
		return true
	}
	_, err := time.Parse(time.RFC3339Nano, *value)
	return err == nil
}
func decodeSummary(data []byte, limit int) (Summary, error) {
	var result Summary
	bad := func() (Summary, error) { return Summary{}, ErrHTTPReceipt }
	if !utf8.Valid(data) || !mcpUnique(data) {
		return bad()
	}
	top, ok := summaryObject(data, []string{"ok", "project", "stats", "recent", "generated_at"}, nil)
	if !ok || !bytes.Equal(bytes.TrimSpace(top["ok"]), []byte("true")) {
		return bad()
	}
	if _, ok = summaryObject(top["project"], []string{"name", "slug"}, []string{"env", "url", "icon"}); !ok {
		return bad()
	}
	stats, ok := summaryObject(top["stats"], []string{"runtimes", "slow_queries", "todos"}, nil)
	if !ok {
		return bad()
	}
	for _, key := range []string{"runtimes", "slow_queries"} {
		if _, ok = summaryObject(stats[key], []string{"open", "total"}, nil); !ok {
			return bad()
		}
	}
	if _, ok = summaryObject(stats["todos"], []string{"open", "in_progress", "done", "total"}, nil); !ok {
		return bad()
	}
	recent, ok := summaryObject(top["recent"], []string{"runtimes", "slow_queries", "todos"}, nil)
	if !ok {
		return bad()
	}
	for _, key := range []string{"runtimes", "slow_queries", "todos"} {
		var rows []json.RawMessage
		if json.Unmarshal(recent[key], &rows) != nil || rows == nil || len(rows) > limit {
			return bad()
		}
		for _, row := range rows {
			var valid bool
			switch key {
			case "runtimes":
				_, valid = summaryObject(row, []string{"hash", "count", "status"}, []string{"exc_class", "exc_message", "last_seen"})
			case "slow_queries":
				_, valid = summaryObject(row, []string{"hash", "max_ms", "count", "status"}, []string{"sql_preview", "last_seen", "at_file", "at_line"})
			case "todos":
				_, valid = summaryObject(row, []string{"id", "title", "status", "priority"}, []string{"created_at"})
			}
			if !valid {
				return bad()
			}
		}
	}
	// Typed projection drops unknown future fields; no arbitrary Cloud error/body escapes.
	if json.Unmarshal(data, &result) != nil {
		return bad()
	}
	if result.Stats.Runtimes.Open < 0 || result.Stats.Runtimes.Total < 0 || result.Stats.SlowQueries.Open < 0 || result.Stats.SlowQueries.Total < 0 || result.Stats.Todos.Open < 0 || result.Stats.Todos.InProgress < 0 || result.Stats.Todos.Done < 0 || result.Stats.Todos.Total < 0 {
		return bad()
	}
	if _, err := time.Parse(time.RFC3339Nano, result.GeneratedAt); err != nil {
		return bad()
	}
	for _, r := range result.Recent.Runtimes {
		if !mcpHash.MatchString(r.Hash) || r.Count < 0 || !summaryRuntimeStatus(r.Status) || !summaryTime(r.LastSeen) {
			return bad()
		}
	}
	for _, r := range result.Recent.SlowQueries {
		if !mcpHash.MatchString(r.Hash) || r.Count < 0 || math.IsNaN(r.MaxMillis) || math.IsInf(r.MaxMillis, 0) || r.MaxMillis < 0 || !summaryRuntimeStatus(r.Status) || (r.Line != nil && *r.Line < 0) || !summaryTime(r.LastSeen) {
			return bad()
		}
	}
	for _, r := range result.Recent.Todos {
		if !mcpTodo.MatchString(r.ID) || (r.Status != "open" && r.Status != "in_progress" && r.Status != "done") || (r.Priority != "low" && r.Priority != "normal" && r.Priority != "high") || !summaryTime(r.CreatedAt) {
			return bad()
		}
	}
	return result, nil
}

func summaryRuntimeStatus(status string) bool {
	return status == "open" || status == "in_progress" || status == "resolved"
}
