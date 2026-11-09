// Package sourcecontext reads optional display context under one explicit source root.
package sourcecontext

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/mask"
	"gitee.com/charsen/moo-monitor-go/records"
)

var ErrConfig = errors.New("source context configuration rejected")
var ErrRead = errors.New("source context unavailable")
var ErrLimit = errors.New("source context limit exceeded")
var ErrClosed = errors.New("source context reader closed")

type Config struct {
	Root                                 string
	AllowedPrefixes                      []string
	Lines, MaxFileBytes, MaxSnippetBytes int
	MaskKeys                             []string
}
type Details struct {
	Frames  []records.Frame
	Snippet records.Snippet
}
type Reader struct {
	mu                         sync.RWMutex
	root                       *os.Root
	path                       string
	prefixes                   []string
	lines, maxFile, maxSnippet int
	masker                     *mask.Masker
}

func New(cfg Config) (*Reader, error) {
	if !supported() || !filepath.IsAbs(cfg.Root) || filepath.Clean(cfg.Root) != cfg.Root || cfg.Root == string(filepath.Separator) || !utf8.ValidString(cfg.Root) {
		return nil, ErrConfig
	}
	if cfg.Lines == 0 {
		cfg.Lines = 10
	}
	if cfg.MaxFileBytes == 0 {
		cfg.MaxFileBytes = 1 << 20
	}
	if cfg.MaxSnippetBytes == 0 {
		cfg.MaxSnippetBytes = 16 << 10
	}
	if cfg.Lines < 0 || cfg.Lines > 100 || cfg.MaxFileBytes < 1 || cfg.MaxFileBytes > 16<<20 || cfg.MaxSnippetBytes < 1 || cfg.MaxSnippetBytes > 64<<10 {
		return nil, ErrConfig
	}
	if len(cfg.AllowedPrefixes) == 0 {
		cfg.AllowedPrefixes = []string{"app", "routes"}
	}
	prefixes := make([]string, 0, len(cfg.AllowedPrefixes))
	for _, prefix := range cfg.AllowedPrefixes {
		prefix = strings.TrimSuffix(prefix, "/")
		if !cleanRelative(prefix) {
			return nil, ErrConfig
		}
		prefixes = append(prefixes, prefix)
	}
	masker, err := mask.New(cfg.MaskKeys)
	if err != nil {
		return nil, ErrConfig
	}
	info, err := os.Lstat(cfg.Root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrConfig
	}
	root, err := os.OpenRoot(cfg.Root)
	if err != nil {
		return nil, ErrConfig
	}
	actual, err := root.Stat(".")
	if err != nil || !os.SameFile(info, actual) {
		root.Close()
		return nil, ErrConfig
	}
	path, err := filepath.EvalSymlinks(cfg.Root)
	if err != nil {
		root.Close()
		return nil, ErrConfig
	}
	return &Reader{root: root, path: path, prefixes: prefixes, lines: cfg.Lines, maxFile: cfg.MaxFileBytes, maxSnippet: cfg.MaxSnippetBytes, masker: masker}, nil
}
func cleanRelative(path string) bool {
	if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\\\x00") || !utf8.ValidString(path) {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		lower := strings.ToLower(part)
		if part == "." || part == ".." || strings.HasPrefix(part, ".") || strings.Contains(lower, "secret") || strings.Contains(lower, "credential") || strings.Contains(lower, "password") || strings.Contains(lower, "token") {
			return false
		}
	}
	return true
}
func (r *Reader) relative(file string) (string, bool) {
	if filepath.IsAbs(file) {
		relative, err := filepath.Rel(r.path, file)
		if err != nil {
			return "", false
		}
		file = relative
	}
	if !cleanRelative(file) || filepath.Ext(file) != ".go" {
		return "", false
	}
	for _, prefix := range r.prefixes {
		if strings.HasPrefix(file, prefix+string(filepath.Separator)) {
			return file, true
		}
	}
	return "", false
}

// Read only enriches display frames. It never computes or replaces exception identity.
func (r *Reader) Read(ctx context.Context, frames []records.Frame) (details Details, failure error) {
	details.Frames = []records.Frame{}
	if ctx == nil {
		return details, ErrConfig
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.root == nil {
		return details, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return details, err
	}
	tried := false
	for _, frame := range frames[:min(len(frames), 64)] {
		if len(details.Frames) == 20 {
			break
		}
		if err := ctx.Err(); err != nil {
			return Details{Frames: details.Frames}, err
		}
		relative, ok := r.relative(frame.File)
		if !ok || frame.Line < 1 || !utf8.ValidString(frame.Function) {
			continue
		}
		lease, err := r.open(relative)
		if err != nil {
			if failure == nil {
				failure = err
			}
			continue
		}
		projected := frame
		projected.File = filepath.ToSlash(relative)
		details.Frames = append(details.Frames, projected)
		if !tried {
			tried = true
			data, err := r.read(ctx, lease)
			if err == nil {
				snippet, err := r.snippet(data, projected)
				if err == nil {
					details.Snippet = snippet
				} else {
					failure = err
				}
			} else {
				failure = err
			}
		}
		if err = lease.close(); err != nil {
			failure = ErrRead
		}
	}
	if failure != nil {
		details.Snippet = records.Snippet{}
	}
	return details, failure
}

type pathStep struct {
	parent *os.Root
	name   string
	info   os.FileInfo
}
type fileLease struct {
	file  *os.File
	roots []*os.Root
	steps []pathStep
	info  os.FileInfo
}

func (l *fileLease) close() error {
	var failure error
	if err := l.file.Close(); err != nil {
		failure = ErrRead
	}
	for i := len(l.roots) - 1; i >= 0; i-- {
		if err := l.roots[i].Close(); err != nil {
			failure = ErrRead
		}
	}
	return failure
}
func (r *Reader) open(relative string) (*fileLease, error) {
	lease := &fileLease{}
	current := r.root
	parts := strings.Split(relative, string(filepath.Separator))
	fail := func(err error) (*fileLease, error) {
		for i := len(lease.roots) - 1; i >= 0; i-- {
			lease.roots[i].Close()
		}
		return nil, err
	}
	for _, part := range parts[:len(parts)-1] {
		info, err := current.Lstat(part)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fail(ErrRead)
		}
		next, err := current.OpenRoot(part)
		if err != nil {
			return fail(ErrRead)
		}
		actual, err := next.Stat(".")
		if err != nil || !os.SameFile(info, actual) {
			next.Close()
			return fail(ErrRead)
		}
		lease.steps = append(lease.steps, pathStep{current, part, info})
		lease.roots = append(lease.roots, next)
		current = next
	}
	name := parts[len(parts)-1]
	info, err := current.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fail(ErrRead)
	}
	if info.Size() > int64(r.maxFile) {
		return fail(ErrLimit)
	}
	file, err := openFile(current, name)
	if err != nil {
		return fail(ErrRead)
	}
	actual, err := file.Stat()
	if err != nil || !os.SameFile(info, actual) || !actual.Mode().IsRegular() {
		file.Close()
		return fail(ErrRead)
	}
	lease.file = file
	lease.info = actual
	lease.steps = append(lease.steps, pathStep{current, name, info})
	return lease, nil
}
func (r *Reader) read(ctx context.Context, lease *fileLease) ([]byte, error) {
	var result bytes.Buffer
	chunk := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := lease.file.Read(chunk)
		if n > 0 {
			if result.Len()+n > r.maxFile {
				return nil, ErrLimit
			}
			result.Write(chunk[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ErrRead
		}
	}
	actual, err := lease.file.Stat()
	if err != nil || actual.Size() != lease.info.Size() || !actual.ModTime().Equal(lease.info.ModTime()) {
		return nil, ErrRead
	}
	for _, step := range lease.steps {
		current, err := step.parent.Lstat(step.name)
		if err != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(step.info, current) {
			return nil, ErrRead
		}
	}
	data := result.Bytes()
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, ErrRead
	}
	return data, nil
}
func (r *Reader) snippet(data []byte, frame records.Frame) (records.Snippet, error) {
	first, last := max(1, frame.Line-r.lines), frame.Line+r.lines
	line, start, end := 1, 0, len(data)
	found := frame.Line == 1
	for index, value := range data {
		if value != '\n' {
			continue
		}
		if line == last {
			end = index
			break
		}
		line++
		if line == first {
			start = index + 1
		}
		if line == frame.Line {
			found = true
		}
	}
	if !found {
		return records.Snippet{}, ErrRead
	}
	if end-start > r.maxSnippet {
		return records.Snippet{}, ErrLimit
	}
	text := r.masker.MaskSecrets(string(data[start:end]))
	if len(text) > r.maxSnippet || !utf8.ValidString(text) {
		return records.Snippet{}, ErrLimit
	}
	return records.Snippet{File: frame.File, Line: frame.Line, Language: "go", Code: text}, nil
}
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.root == nil {
		return nil
	}
	err := r.root.Close()
	r.root = nil
	if err != nil {
		return ErrRead
	}
	return nil
}
