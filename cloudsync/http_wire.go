package cloudsync

import (
	"bytes"
	"encoding/json"
	"math"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/mask"
	"gitee.com/charsen/moo-monitor-go/records"
)

func validWireString(value string) bool { return utf8.ValidString(value) }

// Validate explicit Record fields before encoding/json could substitute invalid UTF-8.
func validateRecordWire(record records.Record) error {
	texts := []string{record.Hash, record.FirstSeen, record.LastSeen, record.Daily.Date, record.Status, record.Exception.Class, record.Exception.Code, record.Exception.Message, record.Exception.File, record.Request.Method, record.Context.Env, record.Context.Project, record.Context.Go, record.Context.Goravel, record.Context.OccurredAt, record.Trace.Full, record.Snippet.File, record.Snippet.Language, record.Snippet.Code}
	for _, pointer := range []*string{record.ResolvedAt, record.ResolvedBy, record.ResolvedNote, record.Request.URL, record.Request.IP, record.Request.UserID, record.Request.UserName, record.Request.Guard} {
		if pointer != nil {
			texts = append(texts, *pointer)
		}
	}
	for _, previous := range record.Exception.Previous {
		texts = append(texts, previous.Class, previous.Message, previous.File)
	}
	for _, frame := range record.Trace.Frames {
		texts = append(texts, frame.File, frame.Function)
	}
	for _, text := range texts {
		if !validWireString(text) {
			return ErrWire
		}
	}
	if err := validateWireValue(record.Meta, 0); err != nil {
		return err
	}
	return validateWireValue(record.Payload, 0)
}

func validateWireValue(value any, depth int) error {
	if depth > 32 {
		return ErrWire
	} // Produced payload is already bounded to depth five.
	switch item := value.(type) {
	case nil, bool, int, int64:
		return nil
	case float64:
		if math.IsNaN(item) || math.IsInf(item, 0) {
			return ErrWire
		}
		return nil
	case string:
		if !validWireString(item) {
			return ErrWire
		}
		return nil
	case []string:
		for _, entry := range item {
			if !validWireString(entry) {
				return ErrWire
			}
		}
		return nil
	case []any:
		for _, entry := range item {
			if err := validateWireValue(entry, depth+1); err != nil {
				return err
			}
		}
		return nil
	case mask.OrderedObject:
		seen := map[string]bool{}
		for _, entry := range item {
			if !validWireString(entry.Key) || seen[entry.Key] {
				return ErrWire
			}
			seen[entry.Key] = true
			if err := validateWireValue(entry.Value, depth+1); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for key, entry := range item {
			if !validWireString(key) {
				return ErrWire
			}
			if err := validateWireValue(entry, depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return ErrWire // No arbitrary structs/Stringers/MarshalJSON hooks at the wire boundary.
	}
}

func encodeRuntimeBody(token string, batch []buffer.Snapshot) ([]byte, error) {
	values := make([]records.Record, 0, len(batch))
	for _, snapshot := range batch {
		if err := validateRecordWire(snapshot.Record); err != nil {
			return nil, err
		}
		values = append(values, snapshot.Record)
	}
	data, err := json.Marshal(struct {
		Token   string           `json:"token"`
		Records []records.Record `json:"records"`
	}{Token: token, Records: values})
	if err != nil {
		return nil, ErrWire
	}
	return data, nil
}
func encodeDiscardBody(token string) ([]byte, error) {
	data, err := json.Marshal(struct {
		Token string `json:"token"`
		Env   string `json:"env"`
	}{Token: token, Env: "local"})
	if err != nil {
		return nil, ErrWire
	}
	return data, nil
}
func decodeDiscard(data []byte) (int64, error) {
	if !utf8.Valid(data) {
		return 0, ErrHTTPReceipt
	}
	var fields map[string]json.RawMessage
	if err := decodeJSON(data, &fields); err != nil {
		return 0, ErrHTTPReceipt
	}
	if string(bytes.TrimSpace(fields["ok"])) != "true" {
		return 0, ErrHTTPReceipt
	}
	raw, exists := fields["deleted"]
	var deleted int64
	if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &deleted) != nil || deleted < 0 {
		return 0, ErrHTTPReceipt
	}
	return deleted, nil
}
