package cloudsync

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSuppressionSchema(t *testing.T) {
	valid := `{"version":1,"entries":{"aaaaaaaaaaaa":{"digest":"` + strings.Repeat("b", 64) + `","timestamp":"2026-11-02T12:00:00+08:00","bucket":"open"}}}`
	if _, err := decodeSuppression([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	cases := []string{`null`, `{}`, `{"version":1,"entries":null}`, `{"version":null,"entries":{}}`, `{"version":1,"entries":{},"unknown":true}`, `{"version":1,"version":1,"entries":{}}`, strings.Replace(valid, `"bucket":"open"`, `"bucket":"resolved"`, 1), strings.Replace(valid, `"bucket":"open"`, `"bucket":null`, 1), strings.Replace(valid, `"timestamp":"2026-11-02T12:00:00+08:00"`, `"timestamp":"not-a-time"`, 1), strings.Replace(valid, `"digest":"`+strings.Repeat("b", 64)+`"`, `"digest":null`, 1), strings.Replace(valid, `"bucket":"open"`, `"bucket":"open","unknown":0`, 1), strings.Replace(valid, `"bucket":"open"`, `"bucket":"open","bucket":"open"`, 1), strings.Replace(valid, "aaaaaaaaaaaa", "../bad-path", 1), valid + ` {}`, string([]byte{0xff})}
	for i, input := range cases {
		if _, err := decodeSuppression([]byte(input)); err == nil {
			t.Errorf("bad suppression accepted case %d", i)
		}
	}
	ledger, err := decodeSuppression([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = decodeSuppression(data); err != nil {
		t.Fatal("encoded ledger did not round trip")
	}
}
