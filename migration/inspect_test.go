package migration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func fakeRecord(t *testing.T, kind string) []byte {
	t.Helper()
	name := "runtime-records"
	if kind == "slow_sql" {
		name = "slow-query-records"
	}
	data, err := os.ReadFile("../docs/contracts/fixtures/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Build map[string]any `json:"build"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	data, err = yaml.Marshal(fixture.Build)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func place(t *testing.T, root, bucket, name string, data []byte) string {
	t.Helper()
	dir := filepath.Join(root, bucket)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func has(r Report, code string) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}
func TestInspectFixtures(t *testing.T) {
	root := t.TempDir()
	runtime := filepath.Join(root, "runtime")
	slow := filepath.Join(root, "slow")
	a := fakeRecord(t, "runtimes")
	b := fakeRecord(t, "slow_sql")
	p := place(t, runtime, "open", "3e4258636dd8.yaml", a)
	place(t, slow, "open", "38a113eb9039.yaml", b)
	cursor := filepath.Join(root, "cursor.json")
	os.WriteFile(cursor, []byte(`{"runtimes":"2026-10-29T12:34:56+08:00"}`), 0600)
	env := filepath.Join(root, "fake.env")
	os.WriteFile(env, []byte("SCAFFOLD_CLOUD_TOKEN=fake-never-output\nSCAFFOLD_RUNTIME_UNKNOWN=fake-secret\n"), 0600)
	before, _ := os.ReadDir(root)
	r, err := Inspect(context.Background(), Config{runtime, slow, cursor, env})
	if err != nil {
		t.Fatal(err)
	}
	if r.ImportSupported || r.Runtimes.Readable != 1 || r.SlowQueries.Readable != 1 || r.Cursor.Readable != 1 || len(r.Mappings) != 2 || !r.Mappings[0].Supported || r.Mappings[1].Supported {
		t.Fatalf("unexpected safe report: %+v", r)
	}
	encoded, _ := json.Marshal(r)
	if strings.Contains(string(encoded), "fake-") || strings.Contains(string(encoded), root) {
		t.Fatal("report leaked input")
	}
	after, _ := os.ReadDir(root)
	actual, _ := os.ReadFile(p)
	if len(before) != len(after) || string(actual) != string(a) {
		t.Fatal("source mutated")
	}
}
func TestInspectRejects(t *testing.T) {
	for _, tc := range []struct{ name, body, code string }{
		{"malformed", "[", "invalid_yaml"}, {"duplicate", "hash: a\nhash: b", "unsupported_yaml"},
		{"alias", "a: &x [one]\nb: *x", "unsupported_yaml"}, {"utf8", string([]byte{255}), "invalid_utf8"},
		{"envelope", "hash: value", "unsupported_envelope"}, {"depth", strings.Repeat("[", 70) + "0" + strings.Repeat("]", 70), "unsupported_yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			p := place(t, root, "open", "aaaaaaaaaaaa.yaml", []byte(tc.body))
			r, err := Inspect(context.Background(), Config{RuntimeSource: root})
			if !errors.Is(err, ErrInspection) || !has(r, tc.code) {
				t.Fatalf("%+v %v", r, err)
			}
			actual, _ := os.ReadFile(p)
			if string(actual) != tc.body {
				t.Fatal("changed source")
			}
		})
	}
	t.Run("cross bucket", func(t *testing.T) {
		root := t.TempDir()
		data := fakeRecord(t, "runtimes")
		place(t, root, "open", "3e4258636dd8.yaml", data)
		place(t, root, "deleted", "3e4258636dd8.yaml", data)
		r, err := Inspect(context.Background(), Config{RuntimeSource: root})
		if err == nil || !has(r, "cross_bucket_hash") || !has(r, "bucket_status_mismatch") {
			t.Fatal(r, err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		outside := filepath.Join(t.TempDir(), "target")
		os.WriteFile(outside, []byte("never-read"), 0600)
		os.Mkdir(filepath.Join(root, "open"), 0700)
		os.Symlink(outside, filepath.Join(root, "open", "aaaaaaaaaaaa.yaml"))
		r, err := Inspect(context.Background(), Config{RuntimeSource: root})
		if err == nil || !has(r, "unsafe_or_unreadable_file") {
			t.Fatal(r, err)
		}
	})
	t.Run("filename and limit", func(t *testing.T) {
		root := t.TempDir()
		place(t, root, "open", "secret-name.yaml", nil)
		place(t, root, "open", "aaaaaaaaaaaa.yaml", []byte(strings.Repeat("x", maxBytes+1)))
		r, err := Inspect(context.Background(), Config{RuntimeSource: root})
		if err == nil || !has(r, "invalid_filename") || !has(r, "file_limit") {
			t.Fatal(r, err)
		}
	})
	for _, body := range []string{`null`, `{"runtimes":null}`, `{"runtimes":"bad"}`, `{"runtimes":"2026-10-29T00:00:00Z","runtimes":"2026-10-29T00:00:00Z"}`, `{"unknown":"2026-10-29T00:00:00Z"}`, `{} {}`} {
		t.Run("cursor", func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "cursor")
			os.WriteFile(p, []byte(body), 0600)
			r, err := Inspect(context.Background(), Config{CursorFile: p})
			if err == nil || !has(r, "invalid_cursor") {
				t.Fatal(r, err)
			}
			b, _ := os.ReadFile(p)
			if string(b) != body {
				t.Fatal("cursor changed")
			}
		})
	}
}
func TestInspectMissingAndCanceled(t *testing.T) {
	if _, err := Inspect(nil, Config{RuntimeSource: "missing"}); !errors.Is(err, ErrInspection) {
		t.Fatal(err)
	}
	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err := Inspect(canceled, Config{RuntimeSource: filepath.Join(t.TempDir(), "missing")}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	r, err := Inspect(context.Background(), Config{RuntimeSource: filepath.Join(t.TempDir(), "absent")})
	if err != nil || !r.Runtimes.Absent || r.ImportSupported {
		t.Fatal(r, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Inspect(ctx, Config{RuntimeSource: t.TempDir()})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = Inspect(context.Background(), Config{}); err == nil {
		t.Fatal("empty config accepted")
	}
}

func TestInspectMissingGroups(t *testing.T) {
	for _, kind := range []string{"runtimes", "slow_sql"} {
		for _, field := range map[string][]string{"runtimes": {"exception", "trace", "source_snippet"}, "slow_sql": {"sql", "took", "at"}}[kind] {
			for _, nullValue := range []bool{false, true} {
				var value map[string]any
				if err := yaml.Unmarshal(fakeRecord(t, kind), &value); err != nil {
					t.Fatal(err)
				}
				if nullValue {
					value[field] = nil
				} else {
					delete(value, field)
				}
				data, err := yaml.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				hash := value["hash"].(string)
				if got := inspectRecord(data, hash, "open", kind); got != "unsupported_type_group" {
					t.Fatalf("%s %s: %s", kind, field, got)
				}
			}
		}
	}
}
