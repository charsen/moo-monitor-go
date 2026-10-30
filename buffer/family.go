package buffer

import (
	"strings"
	"time"

	"gitee.com/charsen/moo-go-monitor/records"
)

// family is the closed, typed boundary between two record schemas and the same durable machinery.
// It is not a plugin registry; constructors select one fixed namespace and schema.
type family[R, S, V any] interface {
	Hash(S) string
	Build(S, time.Time) (*R, error)
	Refresh(R, S, time.Time, int64) (*R, error)
	State(R) lifecycle
	Apply(*R, lifecycle)
	Encode(R) ([]byte, error)
	Decode([]byte, string) (*R, error)
	Row(string, R) V
}
type lifecycle struct {
	Hash, FirstSeen, LastSeen, Status    string
	Count, DailyCount                    int64
	DailyDate                            string
	ResolvedAt, ResolvedBy, ResolvedNote *string
	Meta                                 map[string]any
}
type runtimeFamily struct{ builder *records.Builder }

func (f runtimeFamily) Hash(input records.Snapshot) string { return f.builder.Hash(input.Exception) }
func (f runtimeFamily) Build(input records.Snapshot, now time.Time) (*records.Record, error) {
	return f.builder.Build(input, now)
}
func (f runtimeFamily) Refresh(existing records.Record, input records.Snapshot, now time.Time, overflow int64) (*records.Record, error) {
	return f.builder.Refresh(existing, input, now, overflow)
}
func (runtimeFamily) State(r records.Record) lifecycle {
	return lifecycle{Hash: r.Hash, FirstSeen: r.FirstSeen, LastSeen: r.LastSeen, Status: r.Status, Count: r.Count, DailyDate: r.Daily.Date, DailyCount: r.Daily.Count, ResolvedAt: r.ResolvedAt, ResolvedBy: r.ResolvedBy, ResolvedNote: r.ResolvedNote, Meta: r.Meta}
}
func (runtimeFamily) Apply(r *records.Record, state lifecycle) {
	r.Hash = state.Hash
	r.FirstSeen = state.FirstSeen
	r.LastSeen = state.LastSeen
	r.Status = state.Status
	r.Count = state.Count
	r.Daily = records.Daily{Date: state.DailyDate, Count: state.DailyCount}
	r.ResolvedAt = state.ResolvedAt
	r.ResolvedBy = state.ResolvedBy
	r.ResolvedNote = state.ResolvedNote
	r.Meta = state.Meta
}
func (runtimeFamily) Encode(r records.Record) ([]byte, error) { return encodeRecord(r) }
func (runtimeFamily) Decode(data []byte, hash string) (*records.Record, error) {
	return decodeRecord(data, hash)
}
func (runtimeFamily) Row(bucket string, r records.Record) Row {
	return Row{Hash: r.Hash, Status: bucket, FirstSeen: r.FirstSeen, LastSeen: r.LastSeen, Count: r.Count, Class: r.Exception.Class, MessageFirstLine: firstLine(r.Exception.Message), File: r.Exception.File, Line: r.Exception.Line, URL: r.Request.URL, Method: r.Request.Method}
}
func firstLine(message string) string {
	token := strings.TrimLeft(message, "\n")
	if newline := strings.IndexByte(token, '\n'); newline >= 0 {
		token = token[:newline]
	}
	if token != "" && token != "0" {
		message = token
	}
	message = strings.Trim(message, " \t\n\r\v\x00")
	if len(message) > 160 {
		message = message[:160]
	}
	return message
}
