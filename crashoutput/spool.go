// Package crashoutput owns explicitly enabled, private runtime crash diagnostics.
package crashoutput

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"unicode/utf8"
)

var ErrConfig = errors.New("crash output configuration rejected")
var ErrStorage = errors.New("private crash output storage unavailable")

// createSpool never installs a runtime hook or opens an existing crash file.
// The caller owns all returned handles and the newly created relative filename.
func createSpool(path string) (*os.Root, *os.File, string, error) {
	root, err := openPrivateRoot(path)
	if err != nil {
		return nil, nil, "", err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		root.Close()
		return nil, nil, "", ErrStorage
	}
	name := "crash-" + hex.EncodeToString(random[:]) + ".log"
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		root.Close()
		return nil, nil, "", ErrStorage
	}
	actual, err := file.Stat()
	if err != nil || !actual.Mode().IsRegular() || actual.Mode().Perm()&0077 != 0 {
		file.Close()
		root.Remove(name)
		root.Close()
		return nil, nil, "", ErrStorage
	}
	return root, file, name, nil
}

func openPrivateRoot(path string) (*os.Root, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) || !utf8.ValidString(path) {
		return nil, ErrConfig
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrConfig
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrStorage
	}
	actual, err := root.Stat(".")
	if err != nil || !os.SameFile(info, actual) {
		root.Close()
		return nil, ErrStorage
	}

	return root, nil
}
