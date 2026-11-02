package buffer

import (
	"bytes"
	"errors"
	"io"
	"math"
	"strconv"

	"gitee.com/charsen/moo-go-monitor/mask"
	"gitee.com/charsen/moo-go-monitor/records"
	"go.yaml.in/yaml/v3"
)

// diskRecord reuses the public typed fields; payload gets a narrow ordered-node adapter.
func EncodeRuntimeRecord(record records.Record) ([]byte, error) {
	var node yaml.Node
	if err := node.Encode(record); err != nil {
		return nil, errors.New("cannot encode runtime record")
	}
	payload, err := encodePayload(record.Payload)
	if err != nil {
		return nil, err
	}
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Value == "payload" {
			node.Content[index+1] = payload
		}
	}
	data, err := yaml.Marshal(&node)
	if err != nil {
		return nil, errors.New("cannot encode runtime YAML")
	}
	return data, nil
}

func DecodeRuntimeRecord(data []byte, hash string) (*records.Record, error) {
	var doc yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&doc); err != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("invalid runtime YAML")
	}
	var tail yaml.Node
	if err := decoder.Decode(&tail); err != io.EOF {
		return nil, errors.New("runtime YAML must contain one document")
	}
	node := doc.Content[0]
	payloadNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	seen := map[string]bool{}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
			return nil, errors.New("invalid runtime YAML keys")
		}
		seen[key.Value] = true
		if key.Value == "payload" {
			payloadNode = node.Content[i+1]
			node.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
		}
	}
	if !seen["hash"] || !seen["payload"] || !seen["meta"] || !seen["exception"] || !seen["daily"] {
		return nil, errors.New("runtime YAML lacks required fields")
	}
	payload, err := decodePayload(payloadNode, 0)
	if err != nil {
		return nil, err
	}
	// KnownFields rejects future extensions instead of silently dropping them on rewrite.
	clean, err := yaml.Marshal(node)
	if err != nil {
		return nil, errors.New("cannot inspect runtime YAML")
	}
	typed := yaml.NewDecoder(bytes.NewReader(clean))
	typed.KnownFields(true)
	var record records.Record
	if err := typed.Decode(&record); err != nil {
		return nil, errors.New("invalid or unsupported runtime record fields")
	}
	if record.Hash != hash || !validHash.MatchString(record.Hash) || record.Count < 1 || record.Daily.Count < 1 || !knownBucket(record.Status) || record.FirstSeen == "" || record.LastSeen == "" {
		return nil, errors.New("runtime record identity or counters invalid")
	}
	record.Payload = payload
	return &record, nil
}

func encodePayload(value any) (*yaml.Node, error) {
	switch item := value.(type) {
	case mask.OrderedObject:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, entry := range item {
			child, err := encodePayload(entry.Value)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.Key}, child)
		}
		return node, nil
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, entry := range item {
			child, err := encodePayload(entry)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	case nil, bool, int64, float64, string:
		var node yaml.Node
		if err := node.Encode(item); err != nil {
			return nil, errors.New("cannot encode payload scalar")
		}
		return &node, nil
	default:
		return nil, errors.New("unsupported stored payload representation")
	}
}

func decodePayload(node *yaml.Node, depth int) (any, error) {
	if depth > 16 {
		return nil, errors.New("stored payload nesting too deep")
	}
	switch node.Kind {
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := decodePayload(child, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	case yaml.MappingNode:
		result := make(mask.OrderedObject, 0, len(node.Content)/2)
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
				return nil, errors.New("invalid stored payload key")
			}
			seen[key.Value] = true
			value, err := decodePayload(node.Content[i+1], depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, mask.Entry{Key: key.Value, Value: value})
		}
		return result, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!null":
			return nil, nil
		case "!!str", "!!binary":
			var text string
			if err := node.Decode(&text); err != nil {
				return nil, errors.New("invalid stored text")
			}
			return text, nil
		case "!!bool":
			var value bool
			if err := node.Decode(&value); err != nil {
				return nil, errors.New("invalid stored boolean")
			}
			return value, nil
		case "!!int":
			var value int64
			if err := node.Decode(&value); err != nil {
				return nil, errors.New("stored integer outside int64")
			}
			return value, nil
		case "!!float":
			var value float64
			if err := node.Decode(&value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, errors.New("invalid stored float")
			}
			return value, nil
		}
	}
	return nil, errors.New("unsupported YAML payload node")
}

func (b *store[R, S, V]) readRecord(bucket, hash string) (*R, error) {
	if !knownBucket(bucket) || !validHash.MatchString(hash) {
		return nil, errors.New("invalid buffer record location")
	}
	data, exists, err := b.read(bucket + "/" + hash + ".yaml")
	if err != nil || !exists {
		return nil, err
	}
	return b.builder.Decode(data, hash)
}
func (b *store[R, S, V]) writeRecord(bucket string, record R) error {
	if !knownBucket(bucket) || !validHash.MatchString(b.builder.State(record).Hash) {
		return errors.New("invalid buffer record location")
	}
	data, err := b.builder.Encode(record)
	if err != nil {
		return err
	}
	return b.atomicWrite(bucket+"/"+b.builder.State(record).Hash+".yaml", data)
}
func (b *store[R, S, V]) find(hash string) (*R, string, error) {
	if !validHash.MatchString(hash) {
		return nil, "", errors.New("invalid buffer hash")
	}
	var first *R
	var found string
	for _, bucket := range buckets {
		record, err := b.readRecord(bucket, hash)
		if err != nil {
			return nil, "", err
		}
		if record != nil && first == nil {
			first, found = record, bucket
		}
	}
	return first, found, nil
}
func (b *store[R, S, V]) readOverflow(hash string) (int64, error) {
	data, exists, err := b.read("overflow/" + hash + ".yaml")
	if err != nil || !exists {
		return 0, err
	}
	value, err := strconv.ParseInt(string(bytes.TrimSpace(data)), 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("invalid buffer overflow")
	}
	return value, nil
}
func (b *store[R, S, V]) writeOverflow(hash string, value int64) error {
	if value < 0 || !validHash.MatchString(hash) {
		return errors.New("invalid buffer overflow")
	}
	if value == 0 {
		return b.remove("overflow/" + hash + ".yaml")
	}
	return b.atomicWrite("overflow/"+hash+".yaml", []byte(strconv.FormatInt(value, 10)+"\n"))
}
