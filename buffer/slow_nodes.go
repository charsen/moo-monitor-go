package buffer

import (
	"errors"

	"go.yaml.in/yaml/v3"
)

// The fixed SQL schema must distinguish a required zero from a missing/null scalar.
// These helpers inspect only this record's known containers; they are not a schema runtime.
func slowObject(node *yaml.Node, names ...string) (map[string]*yaml.Node, error) {
	invalid := errors.New("invalid slow-query record structure")
	if node == nil || node.Kind != yaml.MappingNode || node.Tag != "!!map" {
		return nil, invalid
	}
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	values := make(map[string]*yaml.Node, len(names))
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !allowed[key.Value] || values[key.Value] != nil {
			return nil, invalid
		}
		values[key.Value] = node.Content[i+1]
	}
	if len(values) != len(names) {
		return nil, invalid
	}
	return values, nil
}
func slowScalar(node *yaml.Node, tags ...string) bool {
	if node == nil || node.Kind != yaml.ScalarNode {
		return false
	}
	for _, tag := range tags {
		if node.Tag == tag {
			return true
		}
	}
	return false
}
func validateSlowNodes(root *yaml.Node) error {
	invalid := errors.New("invalid slow-query record structure")
	fields, err := slowObject(root, "hash", "first_seen", "last_seen", "count", "status", "resolved_at", "resolved_by", "resolved_note", "sql", "took", "at", "daily", "context", "request", "meta")
	if err != nil {
		return err
	}
	for _, key := range []string{"hash", "first_seen", "last_seen", "status"} {
		if !slowScalar(fields[key], "!!str") {
			return invalid
		}
	}
	if !slowScalar(fields["count"], "!!int") {
		return invalid
	}
	for _, key := range []string{"resolved_at", "resolved_by", "resolved_note"} {
		if !slowScalar(fields[key], "!!str", "!!null") {
			return invalid
		}
	}
	sql, err := slowObject(fields["sql"], "raw", "last", "raw_bytes", "last_bytes")
	if err != nil {
		return err
	}
	if !slowScalar(sql["raw"], "!!str") || !slowScalar(sql["last"], "!!str") || !slowScalar(sql["raw_bytes"], "!!int") || !slowScalar(sql["last_bytes"], "!!int") {
		return invalid
	}
	took, err := slowObject(fields["took"], "last_ms", "max_ms", "threshold_ms")
	if err != nil {
		return err
	}
	if !slowScalar(took["last_ms"], "!!int", "!!float") || !slowScalar(took["max_ms"], "!!int", "!!float") || !slowScalar(took["threshold_ms"], "!!int") {
		return invalid
	}
	at, err := slowObject(fields["at"], "file", "line", "connection")
	if err != nil {
		return err
	}
	if !slowScalar(at["file"], "!!str") || !slowScalar(at["line"], "!!int") || !slowScalar(at["connection"], "!!str", "!!null") {
		return invalid
	}
	daily, err := slowObject(fields["daily"], "date", "count")
	if err != nil {
		return err
	}
	if !slowScalar(daily["date"], "!!str") || !slowScalar(daily["count"], "!!int") {
		return invalid
	}
	context, err := slowObject(fields["context"], "env", "project", "occurred_at")
	if err != nil {
		return err
	}
	for _, key := range []string{"env", "project", "occurred_at"} {
		if !slowScalar(context[key], "!!str") {
			return invalid
		}
	}
	request, err := slowObject(fields["request"], "method", "url", "ip", "user_id", "user_name")
	if err != nil {
		return err
	}
	if !slowScalar(request["method"], "!!str") {
		return invalid
	}
	for _, key := range []string{"url", "ip", "user_id", "user_name"} {
		if !slowScalar(request[key], "!!str", "!!null") {
			return invalid
		}
	}
	meta, err := slowObject(fields["meta"], "updated_at")
	if err != nil {
		return err
	}
	if !slowScalar(meta["updated_at"], "!!str") {
		return invalid
	}
	return nil
}
