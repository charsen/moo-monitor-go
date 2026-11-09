package capture

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitee.com/charsen/moo-monitor-go/records"
	"gitee.com/charsen/moo-monitor-go/sourcecontext"
)

type brokenSource struct{ panicRead bool }

func (s brokenSource) Read(context.Context, []records.Frame) (sourcecontext.Details, error) {
	if s.panicRead {
		panic("fixed-private-source")
	}
	return sourcecontext.Details{}, errors.New("fixed-private-source")
}
func TestSourceContextIdentityAndFailure(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, "app"), 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "app/example.go")
	if err = os.WriteFile(file, []byte("package app\n// token=fixed-private-source\nfunc Example() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	reader, err := sourcecontext.New(sourcecontext.Config{Root: root, MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	collector, store := setup(t)
	input := Options{Frames: []records.Frame{{File: file, Line: 3, Function: "app.Example"}}}
	event := errors.New("fixture event")
	baseline, err := collector.Capture(context.Background(), event, input)
	if err != nil {
		t.Fatal(err)
	}
	collector.config.SourceReader = reader
	hash, err := collector.Capture(context.Background(), event, input)
	if err != nil || hash != baseline {
		t.Fatal("identity changed", err)
	}
	record, err := store.Get(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if record.Exception.File != file || record.Exception.Line != 3 || record.Snippet.File != "app/example.go" || strings.Contains(record.Snippet.Code, "fixed-private-source") || !strings.Contains(record.Snippet.Code, "token=***") {
		t.Fatal("source projection or identity mismatch")
	}
	input.Frames = []records.Frame{{File: "app/base.go", Line: 3}}
	collector.config.SourceReader = nil
	hash, err = collector.Capture(context.Background(), event, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, panics := range []bool{false, true} {
		collector.config.SourceReader = brokenSource{panicRead: panics}
		scope, err := NewEventScope(2)
		if err != nil {
			t.Fatal(err)
		}
		input.Scope, input.EventID = scope, "source-failure"
		before, err := store.Get(context.Background(), hash)
		if err != nil {
			t.Fatal(err)
		}
		got, err := collector.Capture(context.Background(), event, input)
		if got != hash || !errors.Is(err, ErrSourceContext) || strings.Contains(err.Error(), "fixed-private-source") {
			t.Fatal("unsafe or missing auxiliary diagnostic", err)
		}
		got, err = collector.Capture(context.Background(), event, input)
		if got != hash || err != nil {
			t.Fatal("successful base capture was not deduplicated", err)
		}
		after, err := store.Get(context.Background(), hash)
		if err != nil || after.Count != before.Count+1 {
			t.Fatal("lost base event or duplicate count", err)
		}
		if after.Snippet.Code != "" || len(after.Trace.Frames) != 1 || after.Trace.Frames[0].File != "app/base.go" || after.Exception.File != "app/base.go" || after.Trace.Full == "" {
			t.Fatal("base trace lost or unsafe source kept")
		}
		scope.Close()
	}
}
