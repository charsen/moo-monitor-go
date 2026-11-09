package migration

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"go.yaml.in/yaml/v3"
)

func TestConvertPHPFixtures(t *testing.T) {
	runtime, err := ConvertRuntime(fakeRecord(t, "runtimes"), "3e4258636dd8", "open")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Hash != "3e4258636dd8" || runtime.Count != 1 || runtime.Daily.Count != 1 || runtime.Status != "open" || runtime.Context.Go != "" || runtime.Context.Goravel != "" || runtime.Context.PHP == nil || *runtime.Context.PHP == "" || runtime.Context.Laravel == nil {
		t.Fatal("legacy identity or context changed", runtime)
	}
	encoded, err := buffer.EncodeRuntimeRecord(*runtime)
	if err != nil {
		t.Fatal(err)
	}
	reread, err := buffer.DecodeRuntimeRecord(encoded, runtime.Hash)
	if err != nil || reread.Context.PHP == nil || *reread.Context.PHP != *runtime.Context.PHP {
		t.Fatal("storage roundtrip", err)
	}
	slow, err := ConvertSlow(fakeRecord(t, "slow_sql"), "38a113eb9039", "open", ConversionOptions{MaskKeys: []string{"password", "token"}})
	if err != nil {
		t.Fatal(err)
	}
	if slow.Hash != "38a113eb9039" || slow.Count != 1 || slow.Daily.Count != 1 || slow.Status != "open" {
		t.Fatal("slow lifecycle changed")
	}
	encoded, err = buffer.EncodeSlowRecord(*slow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = buffer.DecodeSlowRecord(encoded, slow.Hash); err != nil {
		t.Fatal(err)
	}
}

func TestConvertStrictAndSQLMask(t *testing.T) {
	for _, kind := range []string{"runtimes", "slow_sql"} {
		for _, mode := range []string{"unknown", "type", "hash", "utf8", "tag"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				var value map[string]any
				if err := yaml.Unmarshal(fakeRecord(t, kind), &value); err != nil {
					t.Fatal(err)
				}
				hash := value["hash"].(string)
				switch mode {
				case "unknown":
					value["future_field"] = "must not drop"
				case "type":
					value["count"] = "1"
				case "hash":
					hash = "aaaaaaaaaaaa"
				}
				encoded, err := yaml.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "utf8" {
					encoded = []byte{255}
				}
				if mode == "tag" {
					encoded = []byte(strings.Replace(string(encoded), "count: 1", "count: !future 1", 1))
				}
				if kind == "runtimes" {
					_, err = ConvertRuntime(encoded, hash, "open")
				} else {
					_, err = ConvertSlow(encoded, hash, "open", ConversionOptions{MaskKeys: []string{"password", "token"}})
				}
				if !errors.Is(err, ErrConversion) {
					t.Fatal("invalid conversion accepted", err)
				}
			})
		}
	}
	var value map[string]any
	if err := yaml.Unmarshal(fakeRecord(t, "slow_sql"), &value); err != nil {
		t.Fatal(err)
	}
	sql := value["sql"].(map[string]any)
	sql["raw"] = "select * from people where password='fixed-import-private-value'"
	sql["last"] = "select * from people where token='fixed-import-private-token'"
	encoded, err := yaml.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	converted, err := ConvertSlow(encoded, "38a113eb9039", "open", ConversionOptions{MaskKeys: []string{"password", "token"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(converted)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "fixed-import-private") || converted.SQL.RawBytes != int64(len(converted.SQL.Raw)) || converted.SQL.LastBytes != int64(len(converted.SQL.Last)) {
		t.Fatal("import SQL bypassed shared mask/storage bytes")
	}
}
