package mask

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type payloadSpec struct {
	Type, Name string
	Value      json.RawMessage
	Entries    []struct {
		Key   string
		Value payloadSpec
	}
	Values []payloadSpec
}

func materializePayload(spec payloadSpec) (any, error) {
	switch spec.Type {
	case "scalar":
		decoder := json.NewDecoder(bytes.NewReader(spec.Value))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if number, ok := value.(json.Number); ok {
			if strings.ContainsAny(string(number), ".eE") {
				return strconv.ParseFloat(string(number), 64)
			}
			return strconv.ParseInt(string(number), 10, 64)
		}
		return value, nil
	case "object":
		entries := make(OrderedObject, 0, len(spec.Entries))
		for _, entry := range spec.Entries {
			value, err := materializePayload(entry.Value)
			if err != nil {
				return nil, err
			}
			entries = append(entries, Entry{entry.Key, value})
		}
		return entries, nil
	case "array":
		values := make([]any, 0, len(spec.Values))
		for _, item := range spec.Values {
			value, err := materializePayload(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	case "opaque-object":
		return OpaqueObject{Name: spec.Name}, nil
	case "opaque-resource":
		return Resource{Name: spec.Name}, nil
	}
	return nil, fmt.Errorf("unknown payload fixture type %q", spec.Type)
}

func TestPayloadFixture(t *testing.T) {
	data, err := os.ReadFile("../docs/contracts/fixtures/sensitive-masker-payload.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name            string
			Keys            []string
			StringCap       *int            `json:"string_cap"`
			Depth           int             `json:"depth"`
			InitialBudget   int             `json:"initial_budget"`
			RemainingBudget int             `json:"remaining_budget"`
			InputSpec       payloadSpec     `json:"input_spec"`
			Output          json.RawMessage `json:"output"`
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("payload fixture has no cases")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			masker, err := New(tc.Keys)
			if err != nil {
				t.Fatal(err)
			}
			input, err := materializePayload(tc.InputSpec)
			if err != nil {
				t.Fatal(err)
			}
			cap := 200
			if tc.StringCap != nil {
				cap = *tc.StringCap
			}
			budget := tc.InitialBudget
			result, err := masker.walkPayload(input, tc.Depth, &budget, cap)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(tc.Output, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) || budget != tc.RemainingBudget {
				t.Fatalf("payload differs; budget=%d want=%d", budget, tc.RemainingBudget)
			}
		})
	}
}

type payloadStringer struct{ called *bool }

func (value payloadStringer) String() string {
	*value.called = true
	return "fixed-fake-value-never-stringified"
}

func TestPayloadInputBoundaries(t *testing.T) {
	masker, err := New([]string{"password"})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	for _, input := range []any{
		payloadStringer{&called},
		map[int]any{1: "unsupported"},
		math.NaN(), math.Inf(1),
		string([]byte{0xff}),
		OpaqueObject{Name: string([]byte{0xff})},
		Resource{Name: string([]byte{0xff})},
		OrderedObject{{"same", "first"}, {"same", "second"}},
		OrderedObject{{"0", "coercion"}},
		map[string]any{"0": "coercion"},
		map[string]any{string([]byte{0xff}): "bad-key"},
		OrderedObject{{"valid", "keep"}, {"invalid", payloadStringer{&called}}},
	} {
		if output, err := masker.MaskPayload(input, 200); err == nil || output != nil {
			t.Fatalf("unsupported input returned partial output: %T", input)
		}
	}
	if called {
		t.Fatal("Stringer was invoked")
	}
	first := map[string]any{"z": "last", "a": "first", "password": "fake"}
	second := make(map[string]any)
	for _, entry := range []Entry{{"password", "fake"}, {"a", "first"}, {"z", "last"}} {
		second[entry.Key] = entry.Value
	}
	var previous string
	for _, input := range []map[string]any{first, second} {
		masked, err := masker.MaskPayload(input, 200)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(masked)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != `{"a":"first","password":"***","z":"last"}` {
			t.Fatalf("native map order unstable: %s", encoded)
		}
		if previous != "" && previous != string(encoded) {
			t.Fatal("insertion order changed native map output")
		}
		previous = string(encoded)
	}
	if first["password"] != "fake" || second["password"] != "fake" {
		t.Fatal("input map mutated")
	}
	ordered, err := masker.MaskPayload(OrderedObject{{"z", "last"}, {"a", "first"}}, 200)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(ordered)
	if err != nil || string(encoded) != `{"z":"last","a":"first"}` {
		t.Fatal("explicit source order lost")
	}
	budget := 1
	cut, err := masker.walkPayload(first, 0, &budget, 200)
	if err != nil {
		t.Fatal(err)
	}
	object, ok := cut.(OrderedObject)
	if !ok || len(object) != 2 || object[0].Key != "a" || object[1].Key != "__truncated__" {
		t.Fatal("native map budget did not use stable sorted keys")
	}
}
