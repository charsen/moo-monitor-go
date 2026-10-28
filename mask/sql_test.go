package mask

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSQLSourceFixture(t *testing.T) {
	data, err := os.ReadFile("../docs/contracts/fixtures/sensitive-masker-sql.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name, Method, Input, Output string
			Keys                        []string
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("SQL source fixture has no cases")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Method != "maskSensitiveSql" {
				t.Fatalf("unexpected SQL fixture method %q", tc.Method)
			}
			masker, err := New(tc.Keys)
			if err != nil {
				t.Fatal(err)
			}
			if got := masker.MaskSensitiveSQL(tc.Input); got != tc.Output {
				t.Fatalf("got %q want %q", got, tc.Output)
			}
		})
	}
}
