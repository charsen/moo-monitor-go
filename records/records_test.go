package records

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type phpObject map[string]any

func (value *phpObject) UnmarshalJSON(data []byte) error {
	if string(data) == "[]" {
		*value = nil
		return nil
	}
	type object map[string]any
	var parsed object
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*value = phpObject(parsed)
	return nil
}

type sourceFixture struct {
	PathCases []struct {
		Base           string `json:"base_path"`
		File, Relative string
	} `json:"path_cases"`

	Keys      []string
	BasePath  string `json:"base_path"`
	HashCases []struct {
		Name, Class, File, Message, Hash string
		Line                             int
		NormalizedBase64                 string `json:"normalized_base64"`
		NormalizedBytes                  int    `json:"normalized_bytes"`
	} `json:"hash_cases"`
	MetaCases []struct {
		Name, Source           string
		Meta, Existing, Output phpObject
	} `json:"meta_cases"`
	Build   json.RawMessage
	SameDay json.RawMessage `json:"same_day_refresh"`
	NextDay json.RawMessage `json:"next_day_refresh"`
}

func loadFixture(t *testing.T) sourceFixture {
	t.Helper()
	data, err := os.ReadFile("../docs/contracts/fixtures/runtime-records.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture sourceFixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func sameJSON(t *testing.T, got any, want json.RawMessage) {
	t.Helper()
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var actual, expected any
	if err = json.Unmarshal(data, &actual); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(want, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("record differs\ngot %s\nwant %s", data, want)
	}
}

func TestRuntimeSourceFixture(t *testing.T) {
	fixture := loadFixture(t)
	builder, err := New(Config{MaskKeys: fixture.Keys, BasePath: fixture.BasePath})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.HashCases {
		t.Run("hash/"+tc.Name, func(t *testing.T) {
			normalized := builder.normalizeMessage(tc.Message)
			if base64.StdEncoding.EncodeToString([]byte(normalized)) != tc.NormalizedBase64 || len(normalized) != tc.NormalizedBytes {
				t.Fatal("normalized bytes differ")
			}
			if builder.Hash(Exception{Class: tc.Class, File: tc.File, Line: tc.Line, Message: tc.Message}) != tc.Hash {
				t.Fatal("hash differs")
			}
		})
	}
	for _, tc := range fixture.PathCases {
		t.Run("path/"+tc.Base+tc.File, func(t *testing.T) {
			pathBuilder, err := New(Config{BasePath: tc.Base})
			if err != nil {
				t.Fatal(err)
			}
			if got := pathBuilder.relPath(tc.File); got != tc.Relative {
				t.Fatalf("path %q != %q", got, tc.Relative)
			}
		})
	}
	for _, tc := range fixture.MetaCases {
		t.Run("meta/"+tc.Name, func(t *testing.T) {
			got, err := builder.runtimeMeta(tc.Source, tc.Meta, tc.Existing)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(tc.Output)
			if err != nil {
				t.Fatal(err)
			}
			sameJSON(t, got, encoded)
		})
	}
	var built Record
	if err = json.Unmarshal(fixture.Build, &built); err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{Exception: built.Exception, Request: built.Request, Context: GoContext{Env: "unknown", Project: "unknown", Go: "fixture-go", Goravel: "fixture-goravel"}, Trace: built.Trace, Snippet: built.Snippet, Payload: []any{}, Source: "http_5xx", Meta: map[string]any{"password": "fake", "attempt": int64(0)}}
	snapshot.Exception.Message = "select * from t where token='fake'"
	for index := range snapshot.Exception.Previous {
		snapshot.Exception.Previous[index].Message = "previous " + string(rune('1'+index)) + " password=fake"
	}
	expectedRecord := func(data json.RawMessage) json.RawMessage {
		var record map[string]any
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		context := record["context"].(map[string]any)
		delete(context, "php")
		delete(context, "laravel")
		context["go"] = "fixture-go"
		context["goravel"] = "fixture-goravel"
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	now, err := time.Parse(time.RFC3339Nano, built.FirstSeen)
	if err != nil {
		t.Fatal(err)
	}
	result, err := builder.Build(snapshot, now)
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, result, expectedRecord(fixture.Build))
	result.Status = "resolved"
	result.ResolvedAt = &built.FirstSeen
	actor, note := "fixture-actor", "fixture note"
	result.ResolvedBy = &actor
	result.ResolvedNote = &note
	before, _ := json.Marshal(result)
	snapshot.Source = "reportable"
	snapshot.Meta = map[string]any{"latest": "Bearer fake"}
	now, _ = time.Parse(time.RFC3339Nano, "2026-10-29T13:00:00.123+08:00")
	refreshed, err := builder.Refresh(*result, snapshot, now, 3)
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, refreshed, expectedRecord(fixture.SameDay))
	after, _ := json.Marshal(result)
	if string(before) != string(after) {
		t.Fatal("refresh mutated old record")
	}
	snapshot.Source = "queue_failed"
	snapshot.Meta = nil
	now, _ = time.Parse(time.RFC3339Nano, "2026-10-30T00:00:00.001+08:00")
	refreshed, err = builder.Refresh(*refreshed, snapshot, now, -2)
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, refreshed, expectedRecord(fixture.NextDay))
}

func TestRuntimeSnapshotBoundaries(t *testing.T) {
	maxTrace, cap := 30, 20
	prefixes := []string{"app/", "src/"}
	builder, err := New(Config{MaskKeys: []string{"token", "password"}, BasePath: "/fixture", TraceMaxBytes: &maxTrace, StringMaxChars: &cap, AppFramePrefixes: prefixes})
	if err != nil {
		t.Fatal(err)
	}
	prefixes[0] = "vendor/" // Constructor owns its configuration.
	url := "/path?token=fake&page=1"
	snapshot := Snapshot{Exception: Exception{Class: "GoFixtureError", Message: "token=fake", File: "/fixture/app/example.go", Line: 7, Previous: []Previous{{Message: "password=fake"}, {}, {}, {}}}, Request: Request{Method: "POST", URL: &url}, Context: GoContext{Env: "testing", Go: "1.27.3"}, Trace: Trace{Full: "token=fake " + strings.Repeat("x", 40)}, Snippet: Snippet{File: "/fixture/app/example.go", Language: "go", Code: "token := fakeSourceValue"}, Payload: map[string]any{"token": "fake", "note": strings.Repeat("x", 30)}, Source: "HTTP_5xx"}
	for i := 0; i < 25; i++ {
		snapshot.Trace.Frames = append(snapshot.Trace.Frames, Frame{File: "/fixture/app/example.go", Line: i, Function: "::run::"})
	}
	snapshot.Trace.Frames = append([]Frame{{File: "/fixture/vendor/ignored.go"}}, snapshot.Trace.Frames...)
	now := time.Date(2026, 10, 29, 4, 0, 0, 123456789, time.UTC)
	record, err := builder.Build(snapshot, now)
	if err != nil {
		t.Fatal(err)
	}
	if record.FirstSeen != "2026-10-29T04:00:00.123+00:00" || record.Context.OccurredAt != record.LastSeen {
		t.Fatal("timestamp mismatch")
	}
	if len(record.Trace.Frames) != 20 || record.Trace.Frames[0].File != "app/example.go" || record.Trace.Frames[0].Function != "run" {
		t.Fatal("frames not filtered/limited")
	}
	if record.Trace.Full != "token=*** "+strings.Repeat("x", 20)+"\n... (truncated)" {
		t.Fatalf("trace %q", record.Trace.Full)
	}
	if len(record.Exception.Previous) != 3 || record.Exception.Previous[0].Message != "password=***" {
		t.Fatal("previous chain")
	}
	if *record.Request.URL != "/path?token=***&page=1" || url != *snapshot.Request.URL {
		t.Fatal("URL masking/ownership")
	}
	if record.Snippet.Code != snapshot.Snippet.Code {
		t.Fatal("source snippet changed")
	}
	if record.Meta["source"] != "http_5xx" || record.Meta["updated_at"] != nil {
		t.Fatal("source/storage boundary")
	}
	masked, _ := json.Marshal(record.Payload)
	if !strings.Contains(string(masked), `"token":"***"`) || !strings.Contains(string(masked), `…\u003c+30 chars\u003e`) {
		t.Fatalf("payload %s", masked)
	}
	*record.Request.URL = "changed"
	if url != "/path?token=fake&page=1" {
		t.Fatal("caller URL mutated")
	}
	for _, method := range []string{"GET", "CLI", "HEAD", ""} {
		called := false
		skipped := snapshot
		skipped.Request.Method = method
		skipped.Payload = map[string]any{"token": "fake", "bad": recordStringer{&called}}
		result, err := builder.Build(skipped, now)
		if err != nil {
			t.Fatal(err)
		}
		if values, ok := result.Payload.([]any); !ok || len(values) != 0 || called {
			t.Fatal("read payload was visited")
		}
	}
	emptyWrite := snapshot
	emptyWrite.Payload = nil
	empty, err := builder.Build(emptyWrite, now)
	if err != nil {
		t.Fatal(err)
	}
	if values, ok := empty.Payload.([]any); !ok || len(values) != 0 {
		t.Fatal("nil write payload is not []")
	}
	noBase, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if noBase.relPath("/outside/fake.php") != "/outside/fake.php" {
		t.Fatal("omitted base changed path")
	}
	negative := -2
	negativeBuilder, err := New(Config{TraceMaxBytes: &negative})
	if err != nil {
		t.Fatal(err)
	}
	truncated, err := negativeBuilder.Build(Snapshot{Trace: Trace{Full: "abcdef"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if truncated.Trace.Full != "abcd\n... (truncated)" {
		t.Fatal("negative trace limit")
	}
	bad := snapshot
	bad.Payload = map[string]any{"bad": 1}
	if got, err := builder.Build(bad, now); err == nil || got != nil {
		t.Fatal("unsupported payload leaked partial record")
	}
	bad = snapshot
	bad.Meta = map[string]any{"value": math.Inf(1)}
	if got, err := builder.Build(bad, now); err == nil || got != nil {
		t.Fatal("invalid metadata leaked partial record")
	}
	overflowing := *record
	overflowing.Count = math.MaxInt64
	if got, err := builder.Refresh(overflowing, snapshot, now, 0); err == nil || got != nil {
		t.Fatal("count overflow")
	}
	overflowing = *record
	overflowing.Daily.Count = math.MaxInt64
	if got, err := builder.Refresh(overflowing, snapshot, now, 0); err == nil || got != nil {
		t.Fatal("daily overflow")
	}
	for source, want := range map[string]int{"queue_failed": 30, "schedule_exit": 28, "http_5xx": 25, "log_context": 20, "log_message": 15, "reportable": 10, "custom": 0} {
		if SourceRank(source) != want {
			t.Fatal("source rank")
		}
	}
	changed := snapshot.Exception
	changed.Code = "999"
	changed.Previous = nil
	if builder.Hash(changed) != builder.Hash(snapshot.Exception) {
		t.Fatal("nonidentity fields affected hash")
	}
}

type recordStringer struct{ called *bool }

func (value recordStringer) String() string { *value.called = true; return "fake-do-not-call" }
