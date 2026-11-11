package mask

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSourceFixture(t *testing.T) {
	data, err := os.ReadFile("../docs/contracts/fixtures/sensitive-masker-basic.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name, Method, Input string
			Keys                []string
			Output              json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("source fixture has no cases")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			masker, err := New(tc.Keys)
			if err != nil {
				t.Fatal(err)
			}
			var got any
			switch tc.Method {
			case "shouldMaskKey":
				got = masker.ShouldMaskKey(tc.Input)
			case "maskUrl":
				got = masker.MaskURL(tc.Input)
			case "maskSecrets":
				got = masker.MaskSecrets(tc.Input)
			default:
				t.Fatalf("unknown fixture method %s", tc.Method)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var actual, expected any
			if err := json.Unmarshal(encoded, &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(tc.Output, &expected); err != nil {
				t.Fatal(err)
			}
			// Keep the PHP source fixture unchanged as historical evidence. Go now
			// masks the entire compound Digest header, including its realm.
			if tc.Name == "auth-digest" {
				expected = "Authorization: ***"
			}
			if actual != expected {
				t.Fatalf("got %q want %s", got, tc.Output)
			}
		})
	}
}

func TestConfiguration(t *testing.T) {
	keys := []string{"token"}
	masker, err := New(keys)
	if err != nil {
		t.Fatal(err)
	}
	keys[0] = "password"
	if !masker.ShouldMaskKey("TOKEN") || masker.ShouldMaskKey("password") {
		t.Fatal("caller mutated masker configuration")
	}
	if result, err := New([]string{string([]byte{0xff})}); err == nil || result != nil {
		t.Fatal("invalid UTF-8 configuration accepted")
	}
}
