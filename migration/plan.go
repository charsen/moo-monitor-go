package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/buffer"
)

var ErrPlan = errors.New("migration plan rejected")
var scopeName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Sources struct{ RuntimeSource, SlowSource, CursorFile string }
type Options struct {
	TargetRoot, Scope string
	MaskKeys          []string
}
type PlannedRecord struct{ Family, Bucket, Hash, SourceDigest, TargetDigest string }
type plannedEntry struct {
	PlannedRecord
	SourceRoot, Relative string
	Data                 []byte
}
type artifact struct{ Root, Name, Digest string }

type Plan struct {
	Digest    string
	records   []plannedEntry
	artifacts []artifact
	options   Options
	sources   Sources
	fault     func(string, int) error
}

func (p *Plan) Records() []PlannedRecord {
	out := make([]PlannedRecord, len(p.records))
	for i, r := range p.records {
		out[i] = r.PlannedRecord
	}
	return out
}
func (p *Plan) Target() (string, string) { return p.options.TargetRoot, p.options.Scope }

// BuildPlan reads explicit sources only. It never opens or creates the target.
func BuildPlan(ctx context.Context, sources Sources, options Options) (*Plan, error) {
	if ctx == nil || !filepath.IsAbs(options.TargetRoot) || options.TargetRoot != filepath.Clean(options.TargetRoot) || len(options.Scope) > 64 || !scopeName.MatchString(options.Scope) || (sources.RuntimeSource == "" && sources.SlowSource == "") {
		return nil, ErrPlan
	}
	for _, value := range append([]string{options.TargetRoot, options.Scope, sources.RuntimeSource, sources.SlowSource, sources.CursorFile}, options.MaskKeys...) {
		if !utf8.ValidString(value) {
			return nil, ErrPlan
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	options.MaskKeys = append([]string(nil), options.MaskKeys...)
	plan := &Plan{options: options, sources: sources}
	total := 0
	for _, source := range []struct{ family, path string }{{"runtimes", sources.RuntimeSource}, {"slow_sql", sources.SlowSource}} {
		if source.path == "" {
			continue
		}
		if !filepath.IsAbs(source.path) || pathsOverlap(options.TargetRoot, source.path) {
			return nil, ErrPlan
		}
		root, err := openDirectory(source.path)
		if err != nil {
			return nil, ErrPlan
		}
		seen := map[string]bool{}
		for _, bucket := range []string{"open", "resolved", "deleted"} {
			info, err := root.Lstat(bucket)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				root.Close()
				return nil, ErrPlan
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				root.Close()
				return nil, ErrPlan
			}
			directory, err := root.Open(bucket)
			if err != nil {
				root.Close()
				return nil, ErrPlan
			}
			entries, err := directory.ReadDir(10001)
			directory.Close()
			if (err != nil && !errors.Is(err, io.EOF)) || len(entries) > 10000 {
				root.Close()
				return nil, ErrPlan
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			for _, entry := range entries {
				if err := ctx.Err(); err != nil {
					root.Close()
					return nil, err
				}
				if entry.Name() == ".gitignore" || entry.Name() == ".gitkeep" {
					continue
				}
				if len(plan.records) >= 10000 || !hashName.MatchString(entry.Name()) {
					root.Close()
					return nil, ErrPlan
				}
				hash := strings.TrimSuffix(entry.Name(), ".yaml")
				if seen[hash] {
					root.Close()
					return nil, ErrPlan
				}
				seen[hash] = true
				relative := filepath.Join(bucket, entry.Name())
				data, err := readFile(root, relative)
				if err != nil {
					root.Close()
					return nil, ErrPlan
				}
				total += len(data)
				if total > 64<<20 {
					root.Close()
					return nil, ErrPlan
				}
				var encoded []byte
				if source.family == "runtimes" {
					record, err := ConvertRuntime(data, hash, bucket)
					if err != nil {
						root.Close()
						return nil, err
					}
					encoded, err = buffer.EncodeRuntimeRecord(*record)
					if err != nil {
						root.Close()
						return nil, ErrPlan
					}
				} else {
					record, err := ConvertSlow(data, hash, bucket, ConversionOptions{MaskKeys: options.MaskKeys})
					if err != nil {
						root.Close()
						return nil, err
					}
					encoded, err = buffer.EncodeSlowRecord(*record)
					if err != nil {
						root.Close()
						return nil, ErrPlan
					}
				}
				if len(encoded) > maxBytes {
					root.Close()
					return nil, ErrPlan
				}
				plan.records = append(plan.records, plannedEntry{PlannedRecord: PlannedRecord{Family: source.family, Bucket: bucket, Hash: hash, SourceDigest: digest(data), TargetDigest: digest(encoded)}, SourceRoot: source.path, Relative: relative, Data: encoded})
			}
		}
		root.Close()
	}
	if sources.CursorFile != "" {
		if !filepath.IsAbs(sources.CursorFile) {
			return nil, ErrPlan
		}
		root, err := openDirectory(filepath.Dir(sources.CursorFile))
		if err != nil {
			return nil, ErrPlan
		}
		data, err := readFile(root, filepath.Base(sources.CursorFile))
		root.Close()
		if err != nil || !validCursor(data) {
			return nil, ErrPlan
		}
		plan.artifacts = append(plan.artifacts, artifact{Root: filepath.Dir(sources.CursorFile), Name: filepath.Base(sources.CursorFile), Digest: digest(data)})
	}
	var err error
	plan.Digest, err = plan.seal()
	if err != nil {
		return nil, err
	}
	return plan, nil
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func (p *Plan) seal() (string, error) {
	if p == nil {
		return "", ErrPlan
	}
	for _, record := range p.records {
		if digest(record.Data) != record.TargetDigest {
			return "", ErrPlan
		}
	}
	data, err := json.Marshal(struct {
		Version   int
		Options   Options
		Sources   Sources
		Records   []plannedEntry
		Artifacts []artifact
	}{1, p.options, p.sources, p.records, p.artifacts})
	if err != nil {
		return "", ErrPlan
	}
	return digest(data), nil
}

func pathsOverlap(a, b string) bool {
	contains := func(parent, child string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return contains(a, b) || contains(b, a)
}
