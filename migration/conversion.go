package migration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/records"
	"gitee.com/charsen/moo-monitor-go/slowquery"
	"go.yaml.in/yaml/v3"
)

var ErrConversion = errors.New("legacy record conversion rejected")

type ConversionOptions struct{ MaskKeys []string }

// ConvertRuntime retains legacy identity and lifecycle; PHP versions never become Go versions.
func ConvertRuntime(data []byte, hash, bucket string) (*records.Record, error) {
	node, err := conversionNode(data, hash, bucket, "runtimes", reflect.TypeOf(records.Record{}))
	if err != nil {
		return nil, err
	}
	legacy := nodeMap(nodeMap(node)["context"])
	for _, key := range []string{"env", "project", "php", "laravel", "occurred_at"} {
		if legacy[key] == nil || legacy[key].Kind != yaml.ScalarNode || legacy[key].Tag != "!!str" {
			return nil, fmt.Errorf("%w: context.%s", ErrConversion, key)
		}
	}
	for key := range legacy {
		if key != "env" && key != "project" && key != "php" && key != "laravel" && key != "occurred_at" {
			return nil, fmt.Errorf("%w: unsupported context field", ErrConversion)
		}
	}
	record, err := buffer.DecodeRuntimeRecord(data, hash)
	if err != nil {
		return nil, ErrConversion
	}
	if err = validateLifecycle(record.FirstSeen, record.LastSeen, record.Daily.Date, record.Count, record.Daily.Count, record.ResolvedAt, record.Context.OccurredAt); err != nil {
		return nil, err
	}
	if _, err = json.Marshal(record); err != nil {
		return nil, ErrConversion
	}
	return record, nil
}

// ConvertSlow preserves the source hash while applying the current Go SQL storage policy.
func ConvertSlow(data []byte, hash, bucket string, options ConversionOptions) (*slowquery.Record, error) {
	usable := false
	for _, key := range options.MaskKeys {
		if key != "" && key != "0" {
			usable = true
		}
	}
	if !usable {
		return nil, ErrConversion
	}
	node, err := conversionNode(data, hash, bucket, "slow_sql", reflect.TypeOf(slowquery.Record{}))
	if err != nil {
		return nil, err
	}
	// PHP build records can precede the disk writer's updated_at metadata. The Go
	// storage envelope requires it; use the existing lifecycle time, never now.
	fields := nodeMap(node)
	meta := fields["meta"]
	if meta == nil {
		meta = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "meta"}, meta)
	}
	if nodeMap(meta)["updated_at"] == nil {
		meta.Content = append(meta.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "updated_at"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: text(fields["last_seen"])})
	}
	encoded, err := yaml.Marshal(node)
	if err != nil {
		return nil, ErrConversion
	}
	record, err := buffer.DecodeSlowRecord(encoded, hash)
	if err != nil {
		return nil, ErrConversion
	}
	if err = validateLifecycle(record.FirstSeen, record.LastSeen, record.Daily.Date, record.Count, record.Daily.Count, record.ResolvedAt, record.Context.OccurredAt); err != nil {
		return nil, err
	}
	builder, err := slowquery.New(slowquery.Config{Enabled: true, MaskKeys: options.MaskKeys})
	if err != nil {
		return nil, err
	}
	sql, err := builder.SanitizeStoredSQL(record.SQL.Raw, record.SQL.Last)
	if err != nil {
		return nil, ErrConversion
	}
	record.SQL = sql
	if _, err = json.Marshal(record); err != nil {
		return nil, ErrConversion
	}
	return record, nil
}

func conversionNode(data []byte, hash, bucket, kind string, target reflect.Type) (*yaml.Node, error) {
	if len(data) > maxBytes || !utf8.Valid(data) || !hashName.MatchString(hash+".yaml") || (bucket != "open" && bucket != "resolved" && bucket != "deleted") {
		return nil, ErrConversion
	}
	if inspectRecord(data, hash, bucket, kind) != "" {
		return nil, ErrConversion
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&document) != nil {
		return nil, ErrConversion
	}
	var tail yaml.Node
	if decoder.Decode(&tail) != io.EOF {
		return nil, ErrConversion
	}
	if !conversionTags(document.Content[0]) {
		return nil, ErrConversion
	}
	if err := validateTypedNode(document.Content[0], target, ""); err != nil {
		return nil, err
	}
	return document.Content[0], nil
}

// Reflection is limited to verifying the concrete record DTO's YAML shape. It
// does not discover business actions, perform coercion, or execute a schema DSL.
func validateTypedNode(node *yaml.Node, target reflect.Type, path string) error {
	invalid := func() error { return fmt.Errorf("%w: %s", ErrConversion, path) }
	if target.Kind() == reflect.Pointer {
		if node.Tag == "!!null" {
			return nil
		}
		return validateTypedNode(node, target.Elem(), path)
	}
	switch target.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return invalid()
		}
		values := nodeMap(node)
		fields := map[string]reflect.StructField{}
		for i := 0; i < target.NumField(); i++ {
			field := target.Field(i)
			name := strings.Split(field.Tag.Get("yaml"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = field
			}
		}
		for name, value := range values {
			field, ok := fields[name]
			if !ok {
				return invalid()
			}
			if path == "context" && (name == "go" || name == "goravel") {
				return invalid()
			}
			child := name
			if path != "" {
				child = path + "." + name
			}
			if err := validateTypedNode(value, field.Type, child); err != nil {
				return err
			}
		}
		for name, field := range fields {
			if strings.Contains(field.Tag.Get("yaml"), "omitempty") {
				continue
			}
			if path == "context" && (name == "go" || name == "goravel") {
				continue
			}
			if values[name] == nil {
				return fmt.Errorf("%w: missing %s.%s", ErrConversion, path, name)
			}
		}
	case reflect.String:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return invalid()
		}
	case reflect.Int, reflect.Int64, reflect.Int32:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
			return invalid()
		}
	case reflect.Float64:
		if node.Kind != yaml.ScalarNode || (node.Tag != "!!int" && node.Tag != "!!float") {
			return invalid()
		}
	case reflect.Bool:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" {
			return invalid()
		}
	case reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return invalid()
		}
		for _, child := range node.Content {
			if err := validateTypedNode(child, target.Elem(), path+"[]"); err != nil {
				return err
			}
		}
	case reflect.Map:
		if node.Kind != yaml.MappingNode {
			return invalid()
		}
		for i := 1; i < len(node.Content); i += 2 {
			if err := validateTypedNode(node.Content[i], target.Elem(), path+".*"); err != nil {
				return err
			}
		}
	case reflect.Interface:
		// The storage codec validates payload representation; JSON validation rejects nonfinite metadata.
	default:
		return invalid()
	}
	return nil
}
func validateLifecycle(first, last, date string, count, daily int64, resolved *string, occurred string) error {
	if !timestamp(first) || !timestamp(last) || !timestamp(occurred) || count < 1 || daily < 1 {
		return ErrConversion
	}
	if resolved != nil && !timestamp(*resolved) {
		return ErrConversion
	}
	if parsed, err := time.Parse("2006-01-02", date); err != nil || parsed.Format("2006-01-02") != date {
		return ErrConversion
	}
	return nil
}

func conversionTags(node *yaml.Node) bool {
	switch node.Kind {
	case yaml.MappingNode:
		if node.Tag != "!!map" {
			return false
		}
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return false
		}
	case yaml.ScalarNode:
		if node.Tag != "!!str" && node.Tag != "!!int" && node.Tag != "!!float" && node.Tag != "!!bool" && node.Tag != "!!null" {
			return false
		}
	default:
		return false
	}
	for _, child := range node.Content {
		if !conversionTags(child) {
			return false
		}
	}
	return true
}
