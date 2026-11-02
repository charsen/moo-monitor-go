package slowquery

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRenderedSQL(t *testing.T) {
	threshold := float64(0)
	builder, err := New(Config{Enabled: true, ThresholdMillis: &threshold, MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	rendered := "select * from items where token = 'fixed-render-secret' and id = 12"
	input := Input{Raw: "select * from items where token = $1 and id = $2", RenderedSQL: &rendered, TookMillis: 1, File: "app/query.go", Line: 3}
	record, err := builder.Build(input, time.Now())
	if err != nil || strings.Contains(record.SQL.Last, "fixed-render-secret") || !strings.Contains(record.SQL.Last, "***") || record.Hash != builder.Hash(input) {
		t.Fatal("display masking/identity", err)
	}
	other := "select * from items where token = 'another' and id = 99"
	second := input
	second.RenderedSQL = &other
	if builder.Hash(second) != record.Hash {
		t.Fatal("rendered value changed hash")
	}
	for _, bad := range []Input{func() Input { v := input; v.Bindings = []any{1}; return v }(), func() Input { v := input; text := string([]byte{255}); v.RenderedSQL = &text; return v }(), func() Input {
		v := input
		text := strings.Repeat("x", MaxRenderedSQLBytes+1)
		v.RenderedSQL = &text
		return v
	}()} {
		if ok, err := builder.Accept(bad); ok || !errors.Is(err, ErrInput) {
			t.Fatal("bad rendered input accepted", err)
		}
	}
	legacy := Input{Raw: "select ?", Bindings: []any{int64(42)}}
	if last, err := renderedSQL(legacy); err != nil || last != "select 42" {
		t.Fatal("legacy question path changed", err)
	}
	high := float64(100)
	filtered, _ := New(Config{Enabled: true, ThresholdMillis: &high})
	invalid := input
	invalid.RenderedSQL = new(string)
	if ok, err := filtered.Accept(invalid); ok || err != nil {
		t.Fatal("below-threshold display unexpectedly consumed", err)
	}
}
