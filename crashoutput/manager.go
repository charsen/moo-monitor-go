package crashoutput

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"sync"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/mask"
)

var ErrBusy = errors.New("crash output slot already owned")
var ErrClosed = errors.New("crash output manager closed")
var ErrInstall = errors.New("crash output installation failed")
var ErrLimit = errors.New("crash output read limit exceeded")
var ErrRead = errors.New("crash output diagnostic unavailable")
var reportID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var slot struct {
	sync.Mutex
	owner *Owner
}

type Config struct {
	Root         string
	MaxReadBytes int
	MaskKeys     []string
}
type Info struct {
	ID                          string
	Bytes                       int64
	Digest                      string
	OutputPresent, MayBePartial bool
}
type Report struct {
	Info
	MaskedText string
}
type Manager struct {
	mu              sync.RWMutex
	root            *os.Root
	path            string
	maxRead         int
	masker          *mask.Masker
	owner           *Owner
	closing, closed bool
}
type Owner struct {
	manager *Manager
	file    *os.File
	closed  bool
}

// New prepares local diagnostics only. It does not install a process-global hook.
func New(cfg Config) (*Manager, error) {
	if !supported() {
		return nil, ErrConfig
	}
	if cfg.MaxReadBytes == 0 {
		cfg.MaxReadBytes = 1 << 20
	}
	if cfg.MaxReadBytes < 1 || cfg.MaxReadBytes > 16<<20 {
		return nil, ErrConfig
	}
	masker, err := mask.New(cfg.MaskKeys)
	if err != nil {
		return nil, ErrConfig
	}
	root, err := openPrivateRoot(cfg.Root)
	if err != nil {
		return nil, err
	}
	return &Manager{root: root, path: cfg.Root, maxRead: cfg.MaxReadBytes, masker: masker}, nil
}

// Install claims the application's exclusive SetCrashOutput slot. Other packages
// must not use that global slot while this owner is installed; Go has no getter.
func (m *Manager) Install() (*Owner, error) {
	slot.Lock()
	defer slot.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.closing {
		return nil, ErrClosed
	}
	if slot.owner != nil {
		return nil, ErrBusy
	}
	root, file, name, err := createSpool(m.path)
	if err != nil {
		return nil, err
	}
	anchored, firstErr := m.root.Stat(".")
	actual, secondErr := root.Stat(".")
	if firstErr != nil || secondErr != nil || !os.SameFile(anchored, actual) {
		file.Close()
		root.Remove(name)
		root.Close()
		return nil, ErrStorage
	}
	if err = debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
		file.Close()
		root.Remove(name)
		root.Close()
		return nil, ErrInstall
	}
	root.Close()
	owner := &Owner{manager: m, file: file}
	slot.owner = owner
	m.owner = owner
	return owner, nil
}

// Close disables only this package's owned slot, then closes its original handle.
// It does not restore any third-party sink and does not delete crash diagnostics.
func (o *Owner) Close() error {
	slot.Lock()
	defer slot.Unlock()
	if o.closed {
		return nil
	}
	if slot.owner != o {
		return ErrBusy
	}
	if err := debug.SetCrashOutput(nil, debug.CrashOptions{}); err != nil {
		return ErrInstall
	}
	err := o.file.Close()
	o.closed = true
	slot.owner = nil
	o.manager.mu.Lock()
	o.manager.owner = nil
	o.manager.mu.Unlock()
	if err != nil {
		return ErrStorage
	}
	return nil
}
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closing = true
	owner := m.owner
	m.mu.Unlock()
	if owner != nil {
		if err := owner.Close(); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	err := m.root.Close()
	m.closed = true
	if err != nil {
		return ErrStorage
	}
	return nil
}

func (m *Manager) Inspect(ctx context.Context, id string) (Report, error) {
	if ctx == nil || !reportID.MatchString(id) {
		return Report{}, ErrConfig
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.closing {
		return Report{}, ErrClosed
	}
	return m.inspect(ctx, id)
}
func (m *Manager) inspect(ctx context.Context, id string) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	name := "crash-" + id + ".log"
	info, err := m.root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return Report{}, ErrRead
	}
	if info.Size() > int64(m.maxRead) {
		return Report{}, ErrLimit
	}
	file, err := openReport(m.root, name)
	if err != nil {
		return Report{}, ErrRead
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return Report{}, ErrRead
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(m.maxRead)+1))
	if err != nil {
		return Report{}, ErrRead
	}
	if len(data) > m.maxRead {
		return Report{}, ErrLimit
	}
	if err = ctx.Err(); err != nil {
		return Report{}, err
	}
	actual, err = file.Stat()
	current, pathErr := m.root.Lstat(name)
	if err != nil || pathErr != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, current) || actual.Size() != info.Size() || !actual.ModTime().Equal(info.ModTime()) || !utf8.Valid(data) {
		return Report{}, ErrRead
	}
	digest := sha256.Sum256(data)
	return Report{Info: Info{ID: id, Bytes: int64(len(data)), Digest: hex.EncodeToString(digest[:]), OutputPresent: len(data) > 0, MayBePartial: true}, MaskedText: m.masker.MaskSecrets(string(data))}, nil
}
func (m *Manager) List(ctx context.Context) ([]Info, error) {
	if ctx == nil {
		return nil, ErrConfig
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.closing {
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := m.root.Open(".")
	if err != nil {
		return nil, ErrRead
	}
	defer dir.Close()
	entries, err := dir.ReadDir(1001)
	if err != nil && err != io.EOF {
		return nil, ErrRead
	}
	if len(entries) > 1000 {
		return nil, ErrLimit
	}
	result := []Info{}
	for _, entry := range entries {
		name := entry.Name()
		if len(name) != 42 || name[:6] != "crash-" || name[38:] != ".log" || !reportID.MatchString(name[6:38]) {
			continue
		}
		report, err := m.inspect(ctx, name[6:38])
		if err != nil {
			return nil, err
		}
		result = append(result, report.Info)
	}
	return result, nil
}
