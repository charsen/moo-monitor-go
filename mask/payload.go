package mask

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Entry struct {
	Key   string
	Value any
}

// OrderedObject is optional input for preserving source order. Native maps are sorted.
type OrderedObject []Entry

// OpaqueObject and Resource describe type names only; no properties or String methods are read.
type OpaqueObject struct{ Name string }
type Resource struct{ Name string }

// MarshalJSON preserves the order of an already masked object, including truncation markers.
func (object OrderedObject) MarshalJSON() ([]byte, error) {
	var result strings.Builder
	result.WriteByte('{')
	for i, entry := range object {
		key, err := json.Marshal(entry.Key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(entry.Value)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			result.WriteByte(',')
		}
		result.Write(key)
		result.WriteByte(':')
		result.Write(value)
	}
	result.WriteByte('}')
	return []byte(result.String()), nil
}

// MaskPayload cuts branches when source depth exceeds five and shares a 5000-key budget.
// A native map is sorted only when visited; source insertion order cannot be recovered.
// On an encountered unsupported value it returns an error and no partial output.
func (m *Masker) MaskPayload(value any, maxStringChars int) (any, error) {
	budget := 5000
	return m.walkPayload(value, 0, &budget, maxStringChars)
}

func (m *Masker) walkPayload(value any, depth int, budget *int, cap int) (any, error) {
	if depth > 5 {
		return "<deep>", nil
	}
	switch current := value.(type) {
	case nil, bool, int64:
		return current, nil
	case float64:
		if math.IsNaN(current) || math.IsInf(current, 0) {
			return nil, fmt.Errorf("payload float must be finite")
		}
		return current, nil
	case string:
		if !utf8.ValidString(current) {
			return nil, fmt.Errorf("payload string must be valid UTF-8")
		}
		masked := m.MaskSecrets(current)
		length := utf8.RuneCountInString(masked)
		if length <= cap {
			return masked, nil
		}
		keep := 0
		if cap > 20 {
			keep = cap - 20
		}
		prefix, kept := masked, 0
		for index := range masked {
			if kept == keep {
				prefix = masked[:index]
				break
			}
			kept++
		}
		return prefix + "…<+" + strconv.Itoa(length-keep) + " chars>", nil
	case map[string]any:
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		entries := make(OrderedObject, 0, len(keys))
		for _, key := range keys {
			entries = append(entries, Entry{key, current[key]})
		}
		return m.walkPayload(entries, depth, budget, cap)
	case OrderedObject:
		seen := make(map[string]bool, len(current))
		for _, entry := range current {
			if !utf8.ValidString(entry.Key) || seen[entry.Key] {
				return nil, fmt.Errorf("payload object keys must be unique valid UTF-8")
			}
			if number, err := strconv.ParseInt(entry.Key, 10, 64); err == nil && strconv.FormatInt(number, 10) == entry.Key {
				return nil, fmt.Errorf("canonical integer object keys require an explicit array adapter")
			}
			seen[entry.Key] = true
		}
		if len(current) == 0 {
			return []any{}, nil // The source's empty PHP array serializes as [].
		}
		result := make(OrderedObject, 0, min(len(current), max(0, *budget)))
		for _, entry := range current {
			*budget -= 1
			if *budget < 0 {
				return appendPayloadMarker(result), nil
			}
			masked := any("***")
			if !m.ShouldMaskKey(entry.Key) {
				var err error
				masked, err = m.walkPayload(entry.Value, depth+1, budget, cap)
				if err != nil {
					return nil, err
				}
			}
			result = append(result, Entry{entry.Key, masked})
		}
		return result, nil
	case []any:
		result := make([]any, 0, min(len(current), max(0, *budget)))
		for _, item := range current {
			*budget -= 1
			if *budget < 0 {
				object := make(OrderedObject, 0, len(result)+1)
				for index, masked := range result {
					object = append(object, Entry{strconv.Itoa(index), masked})
				}
				return appendPayloadMarker(object), nil
			}
			masked, err := m.walkPayload(item, depth+1, budget, cap)
			if err != nil {
				return nil, err
			}
			result = append(result, masked)
		}
		return result, nil
	case OpaqueObject:
		if !utf8.ValidString(current.Name) {
			return nil, fmt.Errorf("payload object name must be valid UTF-8")
		}
		return "<object:" + current.Name + ">", nil
	case Resource:
		if !utf8.ValidString(current.Name) {
			return nil, fmt.Errorf("payload resource name must be valid UTF-8")
		}
		return "<resource:" + current.Name + ">", nil
	default:
		return nil, fmt.Errorf("unsupported payload type %T", value)
	}
}

func appendPayloadMarker(object OrderedObject) OrderedObject {
	for index := range object {
		if object[index].Key == "__truncated__" {
			object[index].Value = "<too many keys>"
			return object
		}
	}
	return append(object, Entry{"__truncated__", "<too many keys>"})
}
