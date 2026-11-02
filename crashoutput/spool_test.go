package crashoutput

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateSpool(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	first, file, name, err := createSpool(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Mode().Perm() != 0600 || !info.Mode().IsRegular() {
		t.Fatal("unsafe spool", err)
	}
	if info.Size() != 0 {
		t.Fatal("spool is not empty before install")
	}
	second, other, otherName, err := createSpool(root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	defer other.Close()
	if name == otherName {
		t.Fatal("existing crash file reused")
	}
	if _, err = file.WriteString("fixed private fixture"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || string(data) != "fixed private fixture" {
		t.Fatal("owned file mismatch", err)
	}
	for _, mode := range []string{"relative", "missing", "public", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(root, mode)
			switch mode {
			case "relative":
				path = "relative"
			case "public":
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(root, path); err != nil {
					t.Fatal(err)
				}
			}
			r, f, _, err := createSpool(path)
			if r != nil || f != nil || !errors.Is(err, ErrConfig) {
				t.Fatal("unsafe directory accepted", err)
			}
		})
	}
}
