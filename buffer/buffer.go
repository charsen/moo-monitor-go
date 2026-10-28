// Package buffer stores explicitly supplied runtime snapshots in a private local scope.
package buffer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"gitee.com/charsen/moo-go-monitor/records"
	"github.com/gofrs/flock"
)

var (
	ErrFull           = errors.New("runtime open buffer is full")
	ErrOutcomeUnknown = errors.New("runtime buffer outcome unknown; recover before retrying")
	validHash         = regexp.MustCompile(`^[a-f0-9]{12}$`)
	nonSlug           = regexp.MustCompile(`[^a-z0-9]+`)
	buckets           = []string{"open", "resolved", "deleted"}
)

type Config struct {
	Root, Scope       string
	MaxOpen, DailyCap *int
}

type Buffer struct {
	gate              chan struct{}
	root              *os.Root
	path              string
	builder           *records.Builder
	maxOpen, dailyCap int
	// Tests inject at real file-operation boundaries; no public failure switches.
	fault     func(string) error
	writeTest func(*os.File, []byte) (int, error)
}

type Row struct {
	Hash             string  `json:"hash"`
	Status           string  `json:"status"`
	FirstSeen        string  `json:"first_seen"`
	LastSeen         string  `json:"last_seen"`
	Count            int64   `json:"count"`
	Class            string  `json:"class"`
	MessageFirstLine string  `json:"message_first_line"`
	File             string  `json:"file"`
	Line             int     `json:"line"`
	URL              *string `json:"url"`
	Method           string  `json:"method"`
}

// New requires an existing private root owned by the caller. No Host directory is inferred.
func New(config Config, builder *records.Builder) (*Buffer, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, errors.New("buffer currently supports Darwin and Linux only")
	}
	if builder == nil || !filepath.IsAbs(config.Root) {
		return nil, errors.New("buffer requires builder and absolute private root")
	}
	info, err := os.Lstat(config.Root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("buffer root must be an existing private directory")
	}
	for _, bucket := range buckets {
		if _, err := os.Lstat(filepath.Join(config.Root, bucket)); err == nil {
			return nil, errors.New("buffer root must not be an existing bucket directory")
		}
	}
	root, err := os.OpenRoot(config.Root)
	if err != nil {
		return nil, errors.New("cannot open buffer root")
	}
	scope := normalizeScope(config.Scope)
	name := "go-runtime-buffer"
	if scope != "" {
		name += "--" + scope
	}
	if err = ensureDirectory(root, name); err != nil {
		root.Close()
		return nil, err
	}
	scoped, err := root.OpenRoot(name)
	root.Close()
	if err != nil {
		return nil, errors.New("cannot open scoped buffer root")
	}
	// Resolve platform parent aliases such as /var -> /private/var once; all record I/O stays rooted.
	canonical, err := filepath.EvalSymlinks(config.Root)
	if err != nil {
		scoped.Close()
		return nil, errors.New("cannot locate private buffer root")
	}
	b := &Buffer{gate: make(chan struct{}, 1), root: scoped, path: filepath.Join(canonical, name), builder: builder, maxOpen: 500, dailyCap: 10}
	b.gate <- struct{}{}
	if config.MaxOpen != nil {
		b.maxOpen = *config.MaxOpen
	}
	if config.DailyCap != nil {
		b.dailyCap = *config.DailyCap
	}
	if err = b.prepareFiles(); err != nil {
		scoped.Close()
		return nil, err
	}
	return b, nil
}

func normalizeScope(value string) string {
	var lower strings.Builder
	for _, char := range []byte(strings.Trim(value, " \t\n\r\v\x00")) {
		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		lower.WriteByte(char)
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(lower.String(), "-"), "-")
	if len(slug) > 64 {
		slug = slug[:64]
	}
	return slug
}

func (b *Buffer) Close() error {
	if err := b.enter(context.Background()); err != nil {
		return err
	}
	defer b.leave()
	if b.root == nil {
		return nil
	}
	err := b.root.Close()
	b.root = nil
	return err
}

// Record serializes one scope, recovers its intent, decides the change and commits a final image.
func (b *Buffer) Record(ctx context.Context, snapshot records.Snapshot, now time.Time) (string, error) {
	if err := b.enter(ctx); err != nil {
		return "", err
	}
	defer b.leave()
	lock, err := b.lock(ctx)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err = b.recover(); err != nil {
		return "", err
	}
	hash := b.builder.Hash(snapshot.Exception)
	existing, bucket, err := b.find(hash)
	if err != nil {
		return "", err
	}
	overflow, err := b.readOverflow(hash)
	if err != nil {
		return "", err
	}
	finalOverflow := int64(0)
	var final *records.Record
	if existing == nil {
		open, err := b.count("open")
		if err != nil {
			return "", err
		}
		if open >= b.maxOpen {
			return "", ErrFull
		}
		final, err = b.builder.Build(snapshot, now)
		if err != nil {
			return "", err
		}
	} else if bucket == "open" && b.dailyCap > 0 && existing.Daily.Date == now.Format("2006-01-02") && existing.Daily.Count >= int64(b.dailyCap) {
		if overflow == math.MaxInt64 {
			return "", errors.New("buffer overflow counter exhausted")
		}
		finalOverflow = overflow + 1
		// A frozen record remains byte-for-byte untouched; only overflow is committed.
	} else {
		if bucket != "open" {
			existing.Status = "open"
			existing.ResolvedAt = nil
			existing.ResolvedBy = nil
			existing.ResolvedNote = nil
		}
		final, err = b.builder.Refresh(*existing, snapshot, now, overflow)
		if err != nil {
			return "", err
		}
	}
	if final != nil {
		final.Status = "open"
		final.Meta["updated_at"] = iso(now)
	}
	intent := intent{Version: 1, Hash: hash, Target: "open", Record: final, Overflow: finalOverflow}
	// Intent contains a fixed final image, never an operation that increments during replay.
	if err = b.writeIntent(intent); err != nil {
		return "", fmt.Errorf("%w: persist intent: %w", ErrOutcomeUnknown, err)
	}
	if err = b.check("intent-durable"); err != nil {
		return "", fmt.Errorf("%w: intent persisted: %w", ErrOutcomeUnknown, err)
	}
	if intent.Record != nil {
		if err = b.writeRecord("open", *intent.Record); err != nil {
			return "", fmt.Errorf("%w: write target: %w", ErrOutcomeUnknown, err)
		}
		if err = b.check("target-durable"); err != nil {
			return "", fmt.Errorf("%w: target persisted: %w", ErrOutcomeUnknown, err)
		}
		for _, old := range []string{"resolved", "deleted"} {
			if err = b.remove(old + "/" + hash + ".yaml"); err != nil {
				return "", fmt.Errorf("%w: remove old bucket: %w", ErrOutcomeUnknown, err)
			}
		}
		if err = b.check("old-removed"); err != nil {
			return "", fmt.Errorf("%w: old bucket removed: %w", ErrOutcomeUnknown, err)
		}
	}
	if err = b.writeOverflow(hash, intent.Overflow); err != nil {
		return "", fmt.Errorf("%w: write overflow: %w", ErrOutcomeUnknown, err)
	}
	if err = b.check("overflow-durable"); err != nil {
		return "", fmt.Errorf("%w: overflow persisted: %w", ErrOutcomeUnknown, err)
	}
	if err = b.remove("intent.yaml"); err != nil {
		return "", fmt.Errorf("%w: clear intent: %w", ErrOutcomeUnknown, err)
	}
	return hash, nil
}

func iso(now time.Time) string {
	value := now.Format("2006-01-02T15:04:05.000Z07:00")
	if strings.HasSuffix(value, "Z") {
		return strings.TrimSuffix(value, "Z") + "+00:00"
	}
	return value
}

func (b *Buffer) Get(ctx context.Context, hash string) (*records.Record, error) {
	if err := b.enter(ctx); err != nil {
		return nil, err
	}
	defer b.leave()
	if !validHash.MatchString(hash) {
		return nil, errors.New("invalid buffer hash")
	}
	lock, err := b.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err = b.recover(); err != nil {
		return nil, err
	}
	record, bucket, err := b.find(hash)
	if record != nil {
		record.Status = bucket
	}
	return record, err
}

func (b *Buffer) Count(ctx context.Context, status string) (int, error) {
	if err := b.enter(ctx); err != nil {
		return 0, err
	}
	defer b.leave()
	if status != "all" && !knownBucket(status) {
		return 0, errors.New("invalid buffer bucket")
	}
	lock, err := b.lock(ctx)
	if err != nil {
		return 0, err
	}
	defer lock.Close()
	if err = b.recover(); err != nil {
		return 0, err
	}
	return b.count(status)
}

func (b *Buffer) List(ctx context.Context, status string, limit int) ([]Row, error) {
	if err := b.enter(ctx); err != nil {
		return nil, err
	}
	defer b.leave()
	if status != "all" && !knownBucket(status) {
		return nil, errors.New("invalid buffer bucket")
	}
	lock, err := b.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err = b.recover(); err != nil {
		return nil, err
	}
	return b.list(status, limit)
}

func knownBucket(value string) bool {
	return value == "open" || value == "resolved" || value == "deleted"
}
func (b *Buffer) check(stage string) error {
	if b.fault != nil {
		return b.fault(stage)
	}
	return nil
}

func (b *Buffer) lock(ctx context.Context) (*flock.Flock, error) {
	if b.root == nil {
		return nil, errors.New("buffer is closed")
	}
	if ctx == nil {
		return nil, errors.New("buffer requires context")
	}
	if err := b.safe(".buffer.lock", false); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(b.path, ".buffer.lock"), flock.SetFlag(lockFlags()), flock.SetPermissions(0600))
	ok, err := lock.TryLockContext(ctx, 5*time.Millisecond)
	if err != nil || !ok {
		lock.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("cannot lock runtime buffer")
	}
	rooted, err := b.root.Stat(".buffer.lock")
	held, heldErr := lock.Stat()
	if err != nil || heldErr != nil || !os.SameFile(rooted, held) {
		lock.Close()
		return nil, errors.New("buffer lock identity changed")
	}
	return lock, nil
}

func (b *Buffer) enter(ctx context.Context) error {
	if ctx == nil {
		return errors.New("buffer requires context")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.gate:
		if err := ctx.Err(); err != nil {
			b.leave()
			return err
		}
		return nil
	}
}
func (b *Buffer) leave() { b.gate <- struct{}{} }
