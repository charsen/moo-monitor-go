// Package migration inspects explicitly selected legacy PHP artifacts without importing them.
package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

var ErrInspection = errors.New("migration inspection incomplete")
var errLimit = errors.New("inspection limit")

const maxBytes = 4 << 20

var hashName = regexp.MustCompile(`^[a-f0-9]{12}\.yaml$`)
var oldKey = regexp.MustCompile(`^SCAFFOLD_(RUNTIME|SQL_SLOW|CLOUD)_[A-Z_]+$`)

type Config struct{ RuntimeSource, SlowSource, CursorFile, EnvFile string }
type Finding struct {
	Source string
	Code   string
	Count  int
}
type Mapping struct {
	OldKey, GoKey string
	Supported     bool
}
type SourceReport struct {
	Selected, Absent bool
	Files, Readable  int
}
type Report struct {
	ImportSupported       bool
	Runtimes, SlowQueries SourceReport
	Cursor, Environment   SourceReport
	Findings              []Finding
	Mappings              []Mapping
	Blockers              []string
}

// Inspect never mutates artifacts. A readable PHP envelope is not an import authorization.
func Inspect(ctx context.Context, cfg Config) (Report, error) {
	r := Report{Blockers: []string{"php_record_conversion_required", "timestamp_cursor_is_not_digest_ack"}}
	if ctx == nil {
		return r, ErrInspection
	}
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	if cfg.RuntimeSource == "" && cfg.SlowSource == "" && cfg.CursorFile == "" && cfg.EnvFile == "" {
		return r, ErrInspection
	}
	add := func(source, code string) {
		for i := range r.Findings {
			if r.Findings[i].Source == source && r.Findings[i].Code == code {
				r.Findings[i].Count++
				return
			}
		}
		r.Findings = append(r.Findings, Finding{source, code, 1})
	}
	for _, source := range []struct {
		name, path string
		out        *SourceReport
	}{{"runtimes", cfg.RuntimeSource, &r.Runtimes}, {"slow_sql", cfg.SlowSource, &r.SlowQueries}} {
		if source.path == "" {
			continue
		}
		source.out.Selected = true
		root, err := openDirectory(source.path)
		if errors.Is(err, os.ErrNotExist) {
			source.out.Absent = true
			continue
		}
		if err != nil {
			add(source.name, "unsafe_or_unreadable_directory")
			continue
		}
		seen := map[string]bool{}
		for _, bucket := range []string{"open", "resolved", "deleted"} {
			if ctx.Err() != nil {
				root.Close()
				return r, ctx.Err()
			}
			info, err := root.Lstat(bucket)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				add(source.name, "unsafe_bucket")
				continue
			}
			dir, err := root.Open(bucket)
			if err != nil {
				add(source.name, "unreadable_bucket")
				continue
			}
			entries, err := dir.ReadDir(10001)
			dir.Close()
			if err != nil && !errors.Is(err, io.EOF) {
				add(source.name, "unreadable_bucket")
				continue
			}
			if len(entries)+source.out.Files > 10000 {
				add(source.name, "entry_limit")
				continue
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			for _, entry := range entries {
				if ctx.Err() != nil {
					root.Close()
					return r, ctx.Err()
				}
				if entry.Name() == ".gitignore" || entry.Name() == ".gitkeep" {
					continue
				}
				source.out.Files++
				if !hashName.MatchString(entry.Name()) {
					add(source.name, "invalid_filename")
					continue
				}
				h := strings.TrimSuffix(entry.Name(), ".yaml")
				if seen[h] {
					add(source.name, "cross_bucket_hash")
				}
				seen[h] = true
				data, err := readFile(root, filepath.Join(bucket, entry.Name()))
				if err != nil {
					if errors.Is(err, errLimit) {
						add(source.name, "file_limit")
					} else {
						add(source.name, "unsafe_or_unreadable_file")
					}
					continue
				}
				if code := inspectRecord(data, h, bucket, source.name); code != "" {
					add(source.name, code)
					continue
				}
				source.out.Readable++
			}
		}
		root.Close()
	}
	for _, artifact := range []struct {
		name, path string
		out        *SourceReport
	}{{"cursor", cfg.CursorFile, &r.Cursor}, {"environment", cfg.EnvFile, &r.Environment}} {
		if artifact.path == "" {
			continue
		}
		artifact.out.Selected = true
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		root, err := openDirectory(filepath.Dir(artifact.path))
		if errors.Is(err, os.ErrNotExist) {
			artifact.out.Absent = true
			continue
		}
		if err != nil {
			add(artifact.name, "unsafe_or_unreadable_file")
			continue
		}
		data, err := readFile(root, filepath.Base(artifact.path))
		root.Close()
		if errors.Is(err, os.ErrNotExist) {
			artifact.out.Absent = true
			continue
		}
		if err != nil {
			if errors.Is(err, errLimit) {
				add(artifact.name, "file_limit")
			} else {
				add(artifact.name, "unsafe_or_unreadable_file")
			}
			continue
		}
		artifact.out.Files = 1
		if artifact.name == "cursor" {
			if !validCursor(data) {
				add(artifact.name, "invalid_cursor")
				continue
			}
		} else {
			if !utf8.Valid(data) {
				add(artifact.name, "invalid_utf8")
				continue
			}
			r.Mappings = envMappings(data)
		}
		artifact.out.Readable = 1
	}
	if len(r.Findings) > 0 {
		return r, ErrInspection
	}
	return r, nil
}

func openDirectory(path string) (*os.Root, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInspection
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, ErrInspection
	}
	return root, nil
}
func readFile(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBytes {
		return nil, errLimit
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInspection
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, ErrInspection
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if len(data) > maxBytes {
		return nil, errLimit
	}
	if err != nil {
		return nil, ErrInspection
	}
	return data, nil
}
func inspectRecord(data []byte, hash, bucket, kind string) string {
	if !utf8.Valid(data) {
		return "invalid_utf8"
	}
	var doc yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&doc) != nil || len(doc.Content) != 1 {
		return "invalid_yaml"
	}
	var tail yaml.Node
	if decoder.Decode(&tail) != io.EOF {
		return "invalid_yaml"
	}
	if !validNode(&doc, 0) {
		return "unsupported_yaml"
	}
	m := nodeMap(doc.Content[0])
	if m == nil {
		return "unsupported_envelope"
	}
	for _, k := range []string{"hash", "first_seen", "last_seen", "status", "count", "daily", "context", "request"} {
		if m[k] == nil {
			return "unsupported_envelope"
		}
	}
	if text(m["hash"]) != hash {
		return "hash_mismatch"
	}
	if text(m["status"]) != bucket {
		return "bucket_status_mismatch"
	}
	for _, k := range []string{"first_seen", "last_seen"} {
		if !timestamp(text(m[k])) {
			return "invalid_timestamp"
		}
	}
	if m["count"].Tag != "!!int" || m["count"].Kind != yaml.ScalarNode {
		return "unsupported_envelope"
	}
	for _, k := range []string{"daily", "context", "request"} {
		if nodeMap(m[k]) == nil {
			return "unsupported_envelope"
		}
	}
	groups := []string{"exception", "trace", "source_snippet"}
	if kind == "slow_sql" {
		groups = []string{"sql", "took", "at"}
	}
	for _, k := range groups {
		if nodeMap(m[k]) == nil {
			return "unsupported_type_group"
		}
	}
	return ""
}
func text(n *yaml.Node) string {
	if n != nil && n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
		return n.Value
	}
	return ""
}
func nodeMap(n *yaml.Node) map[string]*yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	m := map[string]*yaml.Node{}
	for i := 0; i < len(n.Content); i += 2 {
		m[n.Content[i].Value] = n.Content[i+1]
	}
	return m
}
func validNode(n *yaml.Node, depth int) bool {
	if depth > 64 {
		return false
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return false
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag != "!!str" || seen[k.Value] {
				return false
			}
			seen[k.Value] = true
		}
	}
	for _, child := range n.Content {
		if !validNode(child, depth+1) {
			return false
		}
	}
	return true
}
func timestamp(s string) bool { _, err := time.Parse(time.RFC3339Nano, s); return err == nil }
func validCursor(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return false
		}
		k, ok := key.(string)
		if !ok || seen[k] || (k != "runtimes" && k != "slow_sql") {
			return false
		}
		seen[k] = true
		var v string
		if d.Decode(&v) != nil || !timestamp(v) {
			return false
		}
	}
	if _, err = d.Token(); err != nil {
		return false
	}
	_, err = d.Token()
	return err == io.EOF
}
func envMappings(data []byte) []Mapping {
	known := map[string]string{"SCAFFOLD_CLOUD_ENABLED": "MONITOR_CLOUD_ENABLED", "SCAFFOLD_CLOUD_URL": "MONITOR_CLOUD_URL", "SCAFFOLD_CLOUD_TOKEN": "MONITOR_CLOUD_TOKEN", "SCAFFOLD_RUNTIME_ENABLED": "MONITOR_ENABLED", "SCAFFOLD_SQL_SLOW_ENABLED": "MONITOR_SLOW_ENABLED", "SCAFFOLD_SQL_SLOW_THRESHOLD_MS": "MONITOR_SLOW_THRESHOLD_MS"}
	found := map[string]bool{}
	out := []Mapping{}
	for _, line := range strings.Split(string(data), "\n") {
		key, _, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !oldKey.MatchString(key) || found[key] {
			continue
		}
		found[key] = true
		target, ok := known[key]
		out = append(out, Mapping{key, target, ok})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OldKey < out[j].OldKey })
	return out
}
