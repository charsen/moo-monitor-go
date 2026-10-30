package slowquery

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-go-monitor/mask"
)

type sourceFixture struct {
	Keys   []string `json:"keys"`
	Base   string   `json:"base_path"`
	Hashes []struct {
		Name, Raw, File string
		Line            int
		Normalized      string `json:"normalized_base64"`
		Hash            string
	} `json:"hash_cases"`
	Bindings []struct {
		Name, Raw string
		Bindings  []struct {
			Type  string
			Value any
		}
		Last string
	} `json:"binding_cases"`
	Rounds      []struct{ Input, Rounded float64 } `json:"round_cases"`
	Truncations []struct {
		Name         string
		Input        string `json:"input_base64"`
		SourceLength int    `json:"source_length"`
		Valid        bool   `json:"output_valid_utf8"`
	} `json:"truncate_cases"`
	Build   Record `json:"build"`
	SameDay Record `json:"same_day_refresh"`
	NextDay Record `json:"next_day_refresh"`
	Unicode Record `json:"unicode_build"`
}

func fixture(t *testing.T) sourceFixture {
	t.Helper()
	data, err := os.ReadFile("../docs/contracts/fixtures/slow-query-records.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value sourceFixture
	if err = decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
func TestSlowQuerySourceFixture(t *testing.T) {
	source := fixture(t)
	builder, err := New(Config{Enabled: true, MaskKeys: source.Keys, BasePath: source.Base, Env: "testing", Project: "fixed-slow-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	for _, example := range source.Hashes {
		t.Run("hash/"+example.Name, func(t *testing.T) {
			normalized := normalizeSQL(example.Raw)
			if base64.StdEncoding.EncodeToString([]byte(normalized)) != example.Normalized || builder.Hash(Input{Raw: example.Raw, File: example.File, Line: example.Line}) != example.Hash {
				t.Fatal("source SQL identity differs")
			}
		})
	}
	for _, example := range source.Bindings {
		t.Run("bindings/"+example.Name, func(t *testing.T) {
			var bindings []any
			for _, binding := range example.Bindings {
				value := binding.Value
				switch binding.Type {
				case "int":
					number, err := strconv.ParseInt(value.(json.Number).String(), 10, 64)
					if err != nil {
						t.Fatal(err)
					}
					value = number
				case "float":
					number, err := value.(json.Number).Float64()
					if err != nil {
						t.Fatal(err)
					}
					value = number
				case "date":
					parsed, err := time.Parse(time.RFC3339, value.(string))
					if err != nil {
						t.Fatal(err)
					}
					value = parsed
				}
				bindings = append(bindings, value)
			}
			text, err := fillBindings(example.Raw, bindings)
			if err != nil || text != example.Last {
				t.Fatal("source binding differs", err)
			}
		})
	}
	for _, example := range source.Rounds {
		if got := roundMillis(example.Input); got != example.Rounded {
			t.Fatalf("source round differs input=%g got=%g want=%g", example.Input, got, example.Rounded)
		}
	}
	now, _ := time.Parse(time.RFC3339Nano, source.Build.LastSeen)
	connection := "fixture-connection"
	input := Input{Raw: "select * from foods where token = 'fake-raw-secret' and id = ?", Bindings: []any{int64(42)}, TookMillis: 100.125, File: source.Base + "/app/query.php", Line: 11, Connection: &connection}
	built, err := builder.Build(input, now)
	if err != nil {
		t.Fatal(err)
	}
	wanted := source.Build
	// Approved local safety differences, derived from source values through the already-tested masker.
	masker, _ := mask.New(source.Keys)
	wanted.SQL.Raw = masker.MaskSecrets(masker.MaskSensitiveSQL(wanted.SQL.Raw))
	wanted.SQL.RawBytes = int64(len(wanted.SQL.Raw))
	wanted.SQL.LastBytes = int64(len(wanted.SQL.Last))
	if !reflect.DeepEqual(*built, wanted) {
		t.Fatal("source build projection differs")
	}
	existing := source.Build
	existing.SQL = wanted.SQL
	existing.Status = "resolved"
	existing.ResolvedAt = source.SameDay.ResolvedAt
	existing.ResolvedBy = source.SameDay.ResolvedBy
	existing.ResolvedNote = source.SameDay.ResolvedNote
	input.Raw = "select * from foods where token=? and id=?"
	input.Bindings = []any{"fake-next-secret", int64(43)}
	input.TookMillis = 90
	input.Connection = nil
	now, _ = time.Parse(time.RFC3339Nano, source.SameDay.LastSeen)
	same, err := builder.Refresh(existing, input, now, 3)
	if err != nil {
		t.Fatal(err)
	}
	wanted = source.SameDay
	wanted.SQL.Raw = built.SQL.Raw
	wanted.SQL.RawBytes = built.SQL.RawBytes
	wanted.SQL.LastBytes = int64(len(wanted.SQL.Last))
	if !reflect.DeepEqual(*same, wanted) {
		t.Fatal("source same-day refresh differs")
	}
	input.Raw = "select 1"
	input.Bindings = nil
	input.TookMillis = 200.005
	connection = "next-connection"
	input.Connection = &connection
	now, _ = time.Parse(time.RFC3339Nano, source.NextDay.LastSeen)
	next, err := builder.Refresh(*same, input, now, -2)
	if err != nil {
		t.Fatal(err)
	}
	wanted = source.NextDay
	wanted.SQL.Raw = built.SQL.Raw
	wanted.SQL.RawBytes = built.SQL.RawBytes
	wanted.SQL.LastBytes = int64(len(wanted.SQL.Last))
	if !reflect.DeepEqual(*next, wanted) {
		t.Fatal("source next-day refresh differs")
	}
}
func TestSlowQuerySafetyAndFiltering(t *testing.T) {
	source := fixture(t)
	builder, _ := New(Config{Enabled: true, SkipPatterns: []string{"skip-this"}, MaskKeys: source.Keys})
	input := Input{Raw: "select ?", Bindings: []any{"中文"}, TookMillis: 100}
	if ok, err := builder.Accept(input); err != nil || !ok {
		t.Fatal("threshold equality skipped")
	}
	input.TookMillis = 99.99
	if ok, _ := builder.Accept(input); ok {
		t.Fatal("below threshold accepted")
	}
	input.TookMillis = 100
	input.Raw = "skip-this"
	if ok, _ := builder.Accept(input); ok {
		t.Fatal("ignore failed")
	}
	input.Raw = "SKIP-THIS"
	if ok, _ := builder.Accept(input); !ok {
		t.Fatal("ignore case broadened")
	}
	for _, example := range source.Truncations {
		original, err := base64.StdEncoding.DecodeString(example.Input)
		if err != nil {
			t.Fatal(err)
		}
		input.Raw = string(original)
		input.Bindings = nil
		built, err := builder.Build(input, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if !utf8.ValidString(built.SQL.Raw) || built.SQL.RawBytes != int64(len(original)) {
			t.Fatal("safe UTF8 byte length differs")
		}
		expected := utf8.RuneCount(original)
		if expected > 4096 {
			expected = 4097
		}
		if utf8.RuneCountInString(built.SQL.Raw) != expected {
			t.Fatal("display rune cap differs")
		}
	}
	for _, bad := range []Input{{Raw: string([]byte{255}), TookMillis: 100}, {Raw: "select ?", Bindings: []any{string([]byte{255})}, TookMillis: 100}, {Raw: "select 1", TookMillis: math.Inf(1)}, {Raw: "select 1", TookMillis: math.NaN()}, {Raw: "select 1", TookMillis: -1}, {Raw: "select ?", Bindings: []any{struct{}{}}, TookMillis: 100}} {
		if record, err := builder.Build(bad, time.Now()); err == nil || record != nil {
			t.Fatal("invalid input produced output")
		}
	}
	input = Input{Raw: "select * from t where token='fake-token-secret'", TookMillis: 100}
	record, err := builder.Build(input, time.Now())
	if err != nil || strings.Contains(record.SQL.Raw, "fake-token-secret") || strings.Contains(record.SQL.Last, "fake-token-secret") {
		t.Fatal("raw/last local safety failed")
	}
	if record.Hash != builder.Hash(input) {
		t.Fatal("mask changed identity")
	}
}

func TestSlowQueryFractionalThreshold(t *testing.T) {
	threshold := 100.5
	builder, err := New(Config{Enabled: true, ThresholdMillis: &threshold})
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Raw: "select 1", TookMillis: 100.49}
	if accepted, err := builder.Accept(input); err != nil || accepted {
		t.Fatal("fractional threshold rounded down")
	}
	input.TookMillis = 100.5
	if accepted, err := builder.Accept(input); err != nil || !accepted {
		t.Fatal("threshold equality rejected")
	}
	built, err := builder.Build(input, time.Now())
	if err != nil || built.Took.ThresholdMillis != 100 {
		t.Fatal("source integer record threshold differs")
	}
	zero := float64(0)
	builder, err = New(Config{Enabled: true, ThresholdMillis: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if accepted, _ := builder.Accept(Input{Raw: "select 1", TookMillis: 0}); !accepted {
		t.Fatal("explicit zero threshold lost")
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1), math.Exp2(63)} {
		if _, err := New(Config{ThresholdMillis: &bad}); err == nil {
			t.Fatal("invalid threshold accepted")
		}
	}
}
