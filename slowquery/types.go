// Package slowquery builds explicit slow-query records; it neither executes SQL nor installs hooks.
package slowquery

import "time"

type Config struct {
	Enabled                bool
	ThresholdMillis        *float64
	SkipPatterns, MaskKeys []string
	BasePath, Env, Project string
}
type Input struct {
	Raw        string
	Bindings   []any
	TookMillis float64
	File       string
	Line       int
	Connection *string
	Request    Request
}
type Request struct {
	Method   string  `json:"method" yaml:"method"`
	URL      *string `json:"url" yaml:"url"`
	IP       *string `json:"ip" yaml:"ip"`
	UserID   *string `json:"user_id" yaml:"user_id"`
	UserName *string `json:"user_name" yaml:"user_name"`
}
type Context struct {
	Env        string `json:"env" yaml:"env"`
	Project    string `json:"project" yaml:"project"`
	OccurredAt string `json:"occurred_at" yaml:"occurred_at"`
}
type SQL struct {
	Raw       string `json:"raw" yaml:"raw"`
	Last      string `json:"last" yaml:"last"`
	RawBytes  int64  `json:"raw_bytes" yaml:"raw_bytes"`
	LastBytes int64  `json:"last_bytes" yaml:"last_bytes"`
}
type Took struct {
	LastMillis      float64 `json:"last_ms" yaml:"last_ms"`
	MaxMillis       float64 `json:"max_ms" yaml:"max_ms"`
	ThresholdMillis int64   `json:"threshold_ms" yaml:"threshold_ms"`
}
type At struct {
	File       string  `json:"file" yaml:"file"`
	Line       int     `json:"line" yaml:"line"`
	Connection *string `json:"connection" yaml:"connection"`
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
	Status       string         `json:"status" yaml:"status"`
	ResolvedAt   *string        `json:"resolved_at" yaml:"resolved_at"`
	ResolvedBy   *string        `json:"resolved_by" yaml:"resolved_by"`
	ResolvedNote *string        `json:"resolved_note" yaml:"resolved_note"`
	SQL          SQL            `json:"sql" yaml:"sql"`
	Took         Took           `json:"took" yaml:"took"`
	At           At             `json:"at" yaml:"at"`
	Daily        Daily          `json:"daily" yaml:"daily"`
	Context      Context        `json:"context" yaml:"context"`
	Request      Request        `json:"request" yaml:"request"`
	Meta         map[string]any `json:"meta,omitempty" yaml:"meta,omitempty"`
}

func timestamp(now time.Time) string {
	result := now.Format("2006-01-02T15:04:05.000Z07:00")
	if result[len(result)-1:] == "Z" {
		result = result[:len(result)-1] + "+00:00"
	}
	return result
}
