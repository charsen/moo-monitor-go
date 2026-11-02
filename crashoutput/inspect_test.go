package crashoutput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectPrivateReadOnly(t *testing.T) {
	for _, mode := range []string{"text", "empty", "oversize", "utf8", "permissions", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			dir, file, name, err := createSpool(root)
			if err != nil {
				t.Fatal(err)
			}
			defer dir.Close()
			data := []byte("panic: token=fixed-inspect-private\n")
			switch mode {
			case "empty":
				data = nil
			case "oversize":
				data = bytes.Repeat([]byte("x"), 65)
			case "utf8":
				data = []byte{255}
			}
			if _, err = file.Write(data); err != nil {
				t.Fatal(err)
			}
			file.Close()
			if mode == "permissions" {
				if err = os.Chmod(filepath.Join(root, name), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "symlink" {
				if err = os.Remove(filepath.Join(root, name)); err != nil {
					t.Fatal(err)
				}
				if err = os.Symlink("absent", filepath.Join(root, name)); err != nil {
					t.Fatal(err)
				}
			}
			manager, err := New(Config{Root: root, MaxReadBytes: 64, MaskKeys: []string{"token"}})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			report, err := manager.Inspect(context.Background(), name[6:38])
			expected := error(nil)
			switch mode {
			case "oversize":
				expected = ErrLimit
			case "utf8", "permissions", "symlink":
				expected = ErrRead
			}
			if !errors.Is(err, expected) {
				t.Fatal("inspection category", err)
			}
			if expected != nil {
				if report.MaskedText != "" {
					t.Fatal("partial unsafe text returned")
				}
				return
			}
			if report.Bytes != int64(len(data)) || report.OutputPresent != (len(data) > 0) || !report.MayBePartial || len(report.Digest) != 64 || strings.Contains(report.MaskedText, "fixed-inspect-private") {
				t.Fatal("unsafe metadata/text")
			}
			after, err := os.ReadFile(filepath.Join(root, name))
			if err != nil || !bytes.Equal(after, data) {
				t.Fatal("raw diagnostic changed", err)
			}
			if _, err = manager.Inspect(context.Background(), "../../outside"); !errors.Is(err, ErrConfig) {
				t.Fatal("arbitrary filename accepted")
			}
			if err = manager.Close(); err != nil {
				t.Fatal(err)
			}
			if err = manager.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err = manager.List(context.Background()); !errors.Is(err, ErrClosed) {
				t.Fatal("closed read allowed")
			}
		})
	}
}
