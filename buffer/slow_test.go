package buffer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-go-monitor/slowquery"
	"go.yaml.in/yaml/v3"
)

func slowTestBuffer(t *testing.T, root, scope string, cap, maxOpen int) *SlowBuffer {
	t.Helper()
	builder, err := slowquery.New(slowquery.Config{Enabled: true, MaskKeys: []string{"token"}, Env: "testing", Project: "slow-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSlow(Config{Root: root, Scope: scope, DailyCap: &cap, MaxOpen: &maxOpen}, builder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}
func slowInput() slowquery.Input {
	return slowquery.Input{Raw: "select * from foods where token=? and id=?", Bindings: []any{"fake-secret", int64(42)}, TookMillis: 100, File: "app/query.go", Line: 11}
}
func reportSlow(t *testing.T, b *SlowBuffer, input slowquery.Input, now time.Time) string {
	t.Helper()
	hash, recorded, err := b.ReportSlowQuery(context.Background(), input, now)
	if err != nil || !recorded {
		t.Fatal("slow report failed", err)
	}
	return hash
}
func TestSlowBufferLifecycle(t *testing.T) {
	root := privateTemp(t)
	b := slowTestBuffer(t, root, "one", 2, 1)
	input := slowInput()
	input.TookMillis = 99.99
	if _, recorded, err := b.ReportSlowQuery(context.Background(), input, testNow); err != nil || recorded {
		t.Fatal("below-threshold recorded")
	}
	input.TookMillis = 100
	hash := reportSlow(t, b, input, testNow)
	input.TookMillis = 120
	reportSlow(t, b, input, testNow.Add(time.Second))
	frozen, _, _ := b.read("open/" + hash + ".yaml")
	for i := 0; i < 3; i++ {
		input.TookMillis = 900
		reportSlow(t, b, input, testNow.Add(time.Duration(i+2)*time.Second))
	}
	after, _, _ := b.read("open/" + hash + ".yaml")
	if !bytes.Equal(frozen, after) {
		t.Fatal("cap changed record")
	}
	input.TookMillis = 150
	reportSlow(t, b, input, testNow.Add(24*time.Hour))
	record, err := b.Get(context.Background(), hash)
	if err != nil || record.Count != 6 || record.Daily.Count != 1 || record.Took.MaxMillis != 150 || record.Took.LastMillis != 150 || bytes.Contains([]byte(record.SQL.Raw), []byte("fake-secret")) || bytes.Contains([]byte(record.SQL.Last), []byte("fake-secret")) {
		t.Fatal("SQL cumulative state", err)
	}
	// Source overflow remembers counts, not frozen-period timing peaks.
	rows, err := b.List(context.Background(), "all", 10)
	if err != nil || len(rows) != 1 || rows[0].MaxMillis != 150 || rows[0].Hash != hash {
		t.Fatal("typed SQL row", err)
	}
	other := slowInput()
	other.Line++
	if _, _, err = b.ReportSlowQuery(context.Background(), other, testNow); !errors.Is(err, ErrFull) {
		t.Fatal("max open gate lost")
	}
	isolated := slowTestBuffer(t, root, "two", 0, 1)
	if reportSlow(t, isolated, input, testNow) != hash {
		t.Fatal("scope changed identity")
	}
	second, _ := isolated.Get(context.Background(), hash)
	if second.Count != 1 {
		t.Fatal("scope state leaked")
	}
	runtime := openBuffer(t, root, "one", 0, 5)
	rHash := mustRecord(t, runtime, testSnapshot(), testNow)
	r := getRecord(t, runtime, rHash)
	r.Hash = hash
	if err = runtime.writeRecord("open", *r); err != nil {
		t.Fatal(err)
	}
	record, _ = b.Get(context.Background(), hash)
	if record.Count != 6 || b.path == runtime.path {
		t.Fatal("type namespace collision")
	}
}
func TestSlowBufferReopen(t *testing.T) {
	for _, bucket := range []string{"resolved", "deleted"} {
		t.Run(bucket, func(t *testing.T) {
			b := slowTestBuffer(t, privateTemp(t), "", 1, 1)
			input := slowInput()
			hash := reportSlow(t, b, input, testNow)
			r, err := b.Get(context.Background(), hash)
			if err != nil {
				t.Fatal(err)
			}
			value := "fixture"
			r.Status = bucket
			r.ResolvedAt = &value
			r.ResolvedBy = &value
			r.ResolvedNote = &value
			if err = b.writeRecord(bucket, *r); err != nil {
				t.Fatal(err)
			}
			if err = b.remove("open/" + hash + ".yaml"); err != nil {
				t.Fatal(err)
			}
			b.maxOpen = 0
			reportSlow(t, b, input, testNow.Add(time.Second))
			r, err = b.Get(context.Background(), hash)
			if err != nil || r.Status != "open" || r.Count != 2 || r.ResolvedAt != nil || r.ResolvedBy != nil || r.ResolvedNote != nil {
				t.Fatal("recurrence did not reopen", err)
			}
			if count, err := b.Count(context.Background(), "all"); err != nil || count != 1 {
				t.Fatal("duplicate bucket", err)
			}
		})
	}
}
func TestSlowBufferRecovery(t *testing.T) {
	for _, stage := range []string{"intent-durable", "target-durable", "old-removed", "overflow-durable", "remove:intent.yaml"} {
		t.Run(stage, func(t *testing.T) {
			root := privateTemp(t)
			b := slowTestBuffer(t, root, "", 0, 10)
			input := slowInput()
			hash := reportSlow(t, b, input, testNow)
			injected := errors.New("fixed slow interruption")
			b.fault = func(point string) error {
				if point == stage {
					return injected
				}
				return nil
			}
			input.TookMillis = 200
			if _, _, err := b.ReportSlowQuery(context.Background(), input, testNow.Add(time.Second)); !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, injected) {
				t.Fatal("cause lost", err)
			}
			b.Close()
			reopened := slowTestBuffer(t, root, "", 0, 10)
			record, err := reopened.Get(context.Background(), hash)
			if err != nil || record.Count != 2 || record.Daily.Count != 2 || record.Took.MaxMillis != 200 {
				t.Fatal("recovered SQL image counted twice", err)
			}
			if _, err = os.Stat(filepath.Join(reopened.path, "intent.yaml")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("intent retained after recovery")
			}
		})
	}
}
func TestSlowBufferCorruption(t *testing.T) {
	for _, kind := range []string{"unknown", "hash", "journal-status"} {
		t.Run(kind, func(t *testing.T) {
			b := slowTestBuffer(t, privateTemp(t), "", 0, 10)
			hash := reportSlow(t, b, slowInput(), testNow)
			before, _, _ := b.read("open/" + hash + ".yaml")
			watched := "open/" + hash + ".yaml"
			bad := before
			switch kind {
			case "unknown":
				bad = append(append([]byte{}, before...), []byte("unsupported_future: true\n")...)
			case "hash":
				bad = bytes.Replace(before, []byte("hash: "+hash), []byte("hash: 000000000000"), 1)
			case "journal-status":
				r, _ := b.Get(context.Background(), hash)
				r.Status = "deleted"
				if err := b.writeIntent(typedIntent[slowquery.Record]{Version: 1, Hash: hash, Target: "open", Record: r, Overflow: 0}); err != nil {
					t.Fatal(err)
				}
				watched = "intent.yaml"
				bad, _, _ = b.read(watched)
			}
			if err := b.root.WriteFile(watched, bad, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := b.Get(context.Background(), hash); err == nil {
				t.Fatal("corruption swallowed")
			}
			retained, _, _ := b.read(watched)
			if !bytes.Equal(bad, retained) {
				t.Fatal("corruption overwritten")
			}
			if kind == "journal-status" {
				current, _, _ := b.read("open/" + hash + ".yaml")
				if !bytes.Equal(before, current) {
					t.Fatal("bad journal changed original")
				}
			}
		})
	}
}

func TestSlowBufferNodePresence(t *testing.T) {
	for _, kind := range []string{"sql-null", "took-missing", "took-null", "line-null"} {
		t.Run(kind, func(t *testing.T) {
			b := slowTestBuffer(t, privateTemp(t), "", 0, 10)
			input := slowInput()
			hash := reportSlow(t, b, input, testNow)
			original, _, _ := b.read("open/" + hash + ".yaml")
			var document yaml.Node
			if err := yaml.Unmarshal(original, &document); err != nil {
				t.Fatal(err)
			}
			child := func(parent *yaml.Node, key string) *yaml.Node {
				for i := 0; i < len(parent.Content); i += 2 {
					if parent.Content[i].Value == key {
						return parent.Content[i+1]
					}
				}
				t.Fatal("fixture node missing")
				return nil
			}
			root := document.Content[0]
			switch kind {
			case "sql-null":
				*child(root, "sql") = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
			case "took-null":
				*child(child(root, "took"), "last_ms") = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
			case "took-missing":
				node := child(root, "took")
				for i := 0; i < len(node.Content); i += 2 {
					if node.Content[i].Value == "last_ms" {
						node.Content = append(node.Content[:i], node.Content[i+2:]...)
						break
					}
				}
			case "line-null":
				*child(child(root, "at"), "line") = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
			}
			corrupt, err := yaml.Marshal(&document)
			if err != nil {
				t.Fatal(err)
			}
			if err = b.writeOverflow(hash, 7); err != nil {
				t.Fatal(err)
			}
			if err = b.root.WriteFile("open/"+hash+".yaml", corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err = b.ReportSlowQuery(context.Background(), input, testNow.Add(time.Second)); err == nil {
				t.Fatal("invalid SQL node accepted")
			}
			retained, _, _ := b.read("open/" + hash + ".yaml")
			if !bytes.Equal(retained, corrupt) {
				t.Fatal("invalid record overwritten")
			}
			if _, exists, _ := b.read("intent.yaml"); exists {
				t.Fatal("invalid record created intent")
			}
			if value, err := b.readOverflow(hash); err != nil || value != 7 {
				t.Fatal("invalid record changed overflow")
			}
		})
	}
	b := slowTestBuffer(t, privateTemp(t), "", 0, 10)
	input := slowInput()
	input.Line = 0
	zero := float64(0)
	builder, err := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &zero})
	if err != nil {
		t.Fatal(err)
	}
	b.builder = builder
	b.store.builder = slowFamily{builder}
	input.TookMillis = 0
	input.Connection = nil
	hash := reportSlow(t, b, input, testNow)
	record, err := b.Get(context.Background(), hash)
	if err != nil || record.At.Line != 0 || record.At.Connection != nil || record.Took.LastMillis != 0 || record.Took.ThresholdMillis != 0 {
		t.Fatal("valid explicit SQL zero/null rejected", err)
	}
}

func TestSlowBufferFrozenInputAndRow(t *testing.T) {
	b := slowTestBuffer(t, privateTemp(t), "", 1, 10)
	input := slowInput()
	hash := reportSlow(t, b, input, testNow)
	before, _, _ := b.read("open/" + hash + ".yaml")
	for _, binding := range []any{string([]byte{255}), struct{}{}} {
		input.Bindings = []any{binding, int64(42)}
		if _, recorded, err := b.ReportSlowQuery(context.Background(), input, testNow.Add(time.Second)); err == nil || recorded {
			t.Fatal("frozen invalid binding recorded")
		}
		after, _, _ := b.read("open/" + hash + ".yaml")
		if !bytes.Equal(before, after) {
			t.Fatal("invalid input changed frozen record")
		}
		if overflow, err := b.readOverflow(hash); err != nil || overflow != 0 {
			t.Fatal("invalid input counted overflow")
		}
		if _, exists, _ := b.read("intent.yaml"); exists {
			t.Fatal("invalid input created intent")
		}
	}
	input = slowInput()
	input.Raw = strings.Repeat("a", 159) + "中"
	input.Bindings = nil
	reportSlow(t, b, input, testNow)
	rows, err := b.List(context.Background(), "all", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if strings.HasPrefix(row.SQLFirstLine, "aaaa") {
			found = true
			if !utf8.ValidString(row.SQLFirstLine) || len(row.SQLFirstLine) != 159 {
				t.Fatal("SQL row cut UTF8")
			}
		}
	}
	if !found {
		t.Fatal("row fixture missing")
	}
}
func TestSlowBufferConcurrent(t *testing.T) {
	root := privateTemp(t)
	one := slowTestBuffer(t, root, "shared", 0, 10)
	two := slowTestBuffer(t, root, "shared", 0, 10)
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Go(func() {
			store := one
			if i%2 == 0 {
				store = two
			}
			if _, recorded, err := store.ReportSlowQuery(context.Background(), slowInput(), testNow); err != nil || !recorded {
				t.Error("concurrent SQL report failed", err)
			}
		})
	}
	group.Wait()
	snapshots, err := one.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 12 || snapshots[0].Record.Daily.Count != 12 {
		t.Fatal("SQL concurrent count lost", err)
	}
}
