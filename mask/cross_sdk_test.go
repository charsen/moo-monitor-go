package mask

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSharedCrossSDKCorpus(t *testing.T) {
	data, err := os.ReadFile("../docs/contracts/fixtures/cross-sdk-redaction.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Keys  []string
		Cases []struct {
			Name, Input, Keep string
			Forbidden         []string
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	m, err := New(fixture.Keys)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			out := m.MaskSecrets(row.Input)
			for _, secret := range row.Forbidden {
				if strings.Contains(out, secret) {
					t.Fatal("credential survived masking")
				}
			}
			if !strings.Contains(out, row.Keep) {
				t.Fatal("safe diagnostic identity was lost")
			}
		})
	}
}

func TestQuotedSecretsRetainNoTail(t *testing.T) {
	m, err := New([]string{"password", "token", "secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`password="first second final-secret" keep=visible`,
		`db_password='first second final-secret' keep=visible`,
		`{"access_token":"first \"second\" final-secret"} keep=visible`,
	} {
		out := m.MaskSecrets(input)
		if strings.Contains(out, "final-secret") || !strings.Contains(out, "keep=visible") {
			t.Fatalf("quoted credential not fully masked: %q", out)
		}
	}
}
