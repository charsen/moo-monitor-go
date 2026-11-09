package buffer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/slowquery"
	"go.yaml.in/yaml/v3"
)

type SlowBuffer struct {
	*store[slowquery.Record, slowquery.Input, SlowRow]
	builder *slowquery.Builder
}
type SlowSnapshot = VersionedSnapshot[slowquery.Record]
type SlowRow struct {
	Hash            string  `json:"hash"`
	Status          string  `json:"status"`
	FirstSeen       string  `json:"first_seen"`
	LastSeen        string  `json:"last_seen"`
	Count           int64   `json:"count"`
	SQLFirstLine    string  `json:"sql_first_line"`
	SQLBytes        int64   `json:"sql_bytes"`
	MaxMillis       float64 `json:"max_ms"`
	LastMillis      float64 `json:"last_ms"`
	ThresholdMillis int64   `json:"threshold_ms"`
	File            string  `json:"file"`
	Line            int     `json:"line"`
	Connection      *string `json:"connection"`
	URL             *string `json:"url"`
	Method          string  `json:"method"`
}

func NewSlow(config Config, builder *slowquery.Builder) (*SlowBuffer, error) {
	if builder == nil {
		return nil, errors.New("buffer requires slow-query builder")
	}
	common, err := newStore(config, slowFamily{builder}, "go-slow-query-buffer")
	if err != nil {
		return nil, err
	}
	return &SlowBuffer{store: common, builder: builder}, nil
}

// ReportSlowQuery is explicit collection, not a database hook. A filtered sample performs no mutation.
func (b *SlowBuffer) ReportSlowQuery(ctx context.Context, input slowquery.Input, now time.Time) (string, bool, error) {
	accepted, err := b.builder.Accept(input)
	if err != nil || !accepted {
		return "", false, err
	}
	hash, err := b.store.record(ctx, input, now)
	return hash, err == nil, err
}

type slowFamily struct{ builder *slowquery.Builder }

func (f slowFamily) Hash(input slowquery.Input) string { return f.builder.Hash(input) }
func (f slowFamily) Build(input slowquery.Input, now time.Time) (*slowquery.Record, error) {
	return f.builder.Build(input, now)
}
func (f slowFamily) Refresh(r slowquery.Record, input slowquery.Input, now time.Time, overflow int64) (*slowquery.Record, error) {
	return f.builder.Refresh(r, input, now, overflow)
}
func (slowFamily) State(r slowquery.Record) lifecycle {
	return lifecycle{Hash: r.Hash, FirstSeen: r.FirstSeen, LastSeen: r.LastSeen, Status: r.Status, Count: r.Count, DailyDate: r.Daily.Date, DailyCount: r.Daily.Count, ResolvedAt: r.ResolvedAt, ResolvedBy: r.ResolvedBy, ResolvedNote: r.ResolvedNote, Meta: r.Meta}
}
func (slowFamily) Apply(r *slowquery.Record, state lifecycle) {
	r.Hash = state.Hash
	r.FirstSeen = state.FirstSeen
	r.LastSeen = state.LastSeen
	r.Status = state.Status
	r.Count = state.Count
	r.Daily = slowquery.Daily{Date: state.DailyDate, Count: state.DailyCount}
	r.ResolvedAt = state.ResolvedAt
	r.ResolvedBy = state.ResolvedBy
	r.ResolvedNote = state.ResolvedNote
	r.Meta = state.Meta
}
func (slowFamily) Row(bucket string, r slowquery.Record) SlowRow {
	return SlowRow{Hash: r.Hash, Status: bucket, FirstSeen: r.FirstSeen, LastSeen: r.LastSeen, Count: r.Count, SQLFirstLine: slowFirstLine(r.SQL.Raw), SQLBytes: r.SQL.RawBytes, MaxMillis: r.Took.MaxMillis, LastMillis: r.Took.LastMillis, ThresholdMillis: r.Took.ThresholdMillis, File: r.At.File, Line: r.At.Line, Connection: r.At.Connection, URL: r.Request.URL, Method: r.Request.Method}
}
func (slowFamily) Encode(record slowquery.Record) ([]byte, error) { return EncodeSlowRecord(record) }
func EncodeSlowRecord(record slowquery.Record) ([]byte, error) {
	data, err := yaml.Marshal(record)
	if err != nil {
		return nil, errors.New("cannot encode slow-query record")
	}
	return data, nil
}
func (slowFamily) Decode(data []byte, hash string) (*slowquery.Record, error) {
	return DecodeSlowRecord(data, hash)
}
func DecodeSlowRecord(data []byte, hash string) (*slowquery.Record, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("invalid slow-query YAML")
	}
	if err := validateSlowNodes(document.Content[0]); err != nil {
		return nil, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var record slowquery.Record
	if err := decoder.Decode(&record); err != nil {
		return nil, errors.New("invalid or unsupported slow-query fields")
	}
	var tail yaml.Node
	if err := decoder.Decode(&tail); err != io.EOF {
		return nil, errors.New("slow-query YAML must contain one document")
	}
	if record.Hash != hash || !validHash.MatchString(hash) || record.Count < 1 || record.Daily.Count < 1 || !knownBucket(record.Status) || record.FirstSeen == "" || record.LastSeen == "" || record.SQL.RawBytes < 0 || record.SQL.LastBytes < 0 || record.At.Line < 0 || record.Took.ThresholdMillis < 0 || record.Took.LastMillis < 0 || record.Took.MaxMillis < record.Took.LastMillis || math.IsNaN(record.Took.LastMillis) || math.IsNaN(record.Took.MaxMillis) || math.IsInf(record.Took.LastMillis, 0) || math.IsInf(record.Took.MaxMillis, 0) {
		return nil, errors.New("invalid slow-query identity or counters")
	}
	for _, text := range []string{record.SQL.Raw, record.SQL.Last, record.At.File, record.Context.Env, record.Context.Project, record.Request.Method} {
		if !utf8.ValidString(text) {
			return nil, errors.New("invalid slow-query text")
		}
	}
	return &record, nil
}

func slowFirstLine(text string) string {
	line := firstLine(text)
	for len(line) > 0 && !utf8.ValidString(line) {
		line = line[:len(line)-1]
	}
	return line
}
