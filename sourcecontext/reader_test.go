package sourcecontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitee.com/charsen/moo-monitor-go/records"
)

func sourceRoot(t *testing.T) string {
	t.Helper()
	if !supported() {
		t.Skip("source reader supports Darwin/Linux")
	}
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(path, "app"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
func sourceFile(t *testing.T, root, name string, data []byte) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
func TestReaderAllowedFramesAndMask(t *testing.T) {
	root := sourceRoot(t)
	sourceFile(t, root, "app/example.go", []byte("package app\n// first\n// token=fixed-source-private\nfunc Example() {}\n// last\n"))
	reader, err := New(Config{Root: root, Lines: 1, MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	frames := []records.Frame{{File: "vendor/external.go", Line: 2}, {File: filepath.Join(root, "app/example.go"), Line: 4, Function: "app.Example"}, {File: "../outside.go", Line: 1}, {File: "app/secret.go", Line: 1}, {File: "app/.hidden.go", Line: 1}}
	result, err := reader.Read(context.Background(), frames)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Frames) != 1 || result.Frames[0].File != "app/example.go" || result.Snippet.File != "app/example.go" || result.Snippet.Line != 4 || result.Snippet.Language != "go" || strings.Contains(result.Snippet.Code, "fixed-source-private") || !strings.Contains(result.Snippet.Code, "token=***") || !strings.Contains(result.Snippet.Code, "func Example") {
		t.Fatal("projection/mask mismatch")
	}
	if frames[1].File != filepath.Join(root, "app/example.go") {
		t.Fatal("input identity changed")
	}
	noMatch, err := reader.Read(context.Background(), []records.Frame{{File: "/not-this-root/outside.go", Line: 1}})
	if err != nil || len(noMatch.Frames) != 0 || noMatch.Snippet.Code != "" {
		t.Fatal("unrelated frame read", err)
	}
	bounded := make([]records.Frame, 65)
	bounded[64] = frames[1]
	result, err = reader.Read(context.Background(), bounded)
	if err != nil || len(result.Frames) != 0 {
		t.Fatal("input scan budget", err)
	}
	repeated := make([]records.Frame, 30)
	for i := range repeated {
		repeated[i] = frames[1]
	}
	result, err = reader.Read(context.Background(), repeated)
	if err != nil || len(result.Frames) != 20 {
		t.Fatal("frame budget", err)
	}
}
func TestReaderRejectsLinksLimitsAndBinary(t *testing.T) {
	for _, mode := range []string{"file-link", "parent-link", "file-limit", "snippet-limit", "binary", "utf8", "range"} {
		t.Run(mode, func(t *testing.T) {
			root := sourceRoot(t)
			name := "app/example.go"
			data := []byte("package app\nfunc Example() {}\n")
			cfg := Config{Root: root}
			switch mode {
			case "file-link", "parent-link":
				outside := t.TempDir()
				sourceFile(t, outside, "example.go", data)
				if mode == "file-link" {
					if err := os.Symlink(filepath.Join(outside, "example.go"), filepath.Join(root, name)); err != nil {
						t.Fatal(err)
					}
				} else {
					name = "app/linked/example.go"
					if err := os.Symlink(outside, filepath.Join(root, "app/linked")); err != nil {
						t.Fatal(err)
					}
				}
			case "file-limit":
				cfg.MaxFileBytes = 4
				sourceFile(t, root, name, data)
			case "snippet-limit":
				cfg.MaxSnippetBytes = 4
				sourceFile(t, root, name, data)
			case "binary":
				sourceFile(t, root, name, []byte{'x', 0, 'x'})
			case "utf8":
				sourceFile(t, root, name, []byte{255})
			default:
				sourceFile(t, root, name, data)
			}
			reader, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			line := 1
			if mode == "range" {
				line = 999
			}
			value, err := reader.Read(context.Background(), []records.Frame{{File: name, Line: line}})
			if err == nil || value.Snippet.Code != "" {
				t.Fatal("unsafe source exposed", mode, err)
			}
			if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), "package app") {
				t.Fatal("error leaked source")
			}
		})
	}
}
func TestReaderConfigurationCancelAndClose(t *testing.T) {
	root := sourceRoot(t)
	for _, prefix := range []string{"../app", "/app", "app/.hidden", "app/credentials", "."} {
		if reader, err := New(Config{Root: root, AllowedPrefixes: []string{prefix}}); err == nil {
			reader.Close()
			t.Fatal("bad prefix accepted", prefix)
		}
	}
	reader, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = reader.Read(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Read(context.Background(), nil); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
