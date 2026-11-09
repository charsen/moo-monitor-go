// Package cloudsync incrementally confirms local runtime snapshots through an explicit transport.
package cloudsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sort"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/buffer"
)

type Item struct {
	Index     int     `json:"index"`
	Hash      string  `json:"hash"`
	Status    string  `json:"status"`
	Retryable bool    `json:"retryable"`
	Reason    *string `json:"reason"`
}
type Receipt struct {
	Saved, Filtered, Skipped int64
	Results                  []Item
	Legacy                   bool
}

// DecodeReceipt validates actual HTTP status and exact wire field presence/types, without I/O.
func DecodeReceipt(status int, data []byte, batch []buffer.Snapshot) (Receipt, error) {
	return decodeReceipt(status, data, batch)
}
func decodeReceipt[R any](status int, data []byte, batch []buffer.VersionedSnapshot[R]) (Receipt, error) {
	invalid := errors.New("invalid Cloud intake receipt")
	if status < 200 || status >= 300 || !utf8.Valid(data) {
		return Receipt{}, invalid
	}
	var fields map[string]json.RawMessage
	if err := decodeJSON(data, &fields); err != nil || fields == nil {
		return Receipt{}, invalid
	}
	if string(bytes.TrimSpace(fields["ok"])) != "true" {
		return Receipt{}, invalid
	}
	receipt := Receipt{}
	for name, destination := range map[string]*int64{"saved": &receipt.Saved, "filtered": &receipt.Filtered, "skipped": &receipt.Skipped} {
		raw, exists := fields[name]
		if !exists || string(bytes.TrimSpace(raw)) == "null" || json.Unmarshal(raw, destination) != nil {
			return Receipt{}, invalid
		}
	}
	raw, present := fields["results"]
	receipt.Legacy = !present
	if present {
		trimmed := bytes.TrimSpace(raw)
		var items []map[string]json.RawMessage
		if len(trimmed) == 0 {
			return Receipt{}, invalid
		}
		switch trimmed[0] {
		case '[':
			if json.Unmarshal(raw, &items) != nil {
				return Receipt{}, invalid
			}
		case '{':
			var values map[string]map[string]json.RawMessage
			if json.Unmarshal(raw, &values) != nil {
				return Receipt{}, invalid
			}
			for _, item := range values {
				items = append(items, item)
			}
		default:
			return Receipt{}, invalid
		}
		for _, fields := range items {
			for _, name := range []string{"index", "hash", "status", "retryable", "reason"} {
				if _, exists := fields[name]; !exists {
					return Receipt{}, invalid
				}
			}
			var item Item
			if string(bytes.TrimSpace(fields["index"])) == "null" || json.Unmarshal(fields["index"], &item.Index) != nil {
				return Receipt{}, invalid
			}
			if string(bytes.TrimSpace(fields["hash"])) == "null" || json.Unmarshal(fields["hash"], &item.Hash) != nil {
				return Receipt{}, invalid
			}
			if string(bytes.TrimSpace(fields["status"])) == "null" || json.Unmarshal(fields["status"], &item.Status) != nil {
				return Receipt{}, invalid
			}
			if string(bytes.TrimSpace(fields["retryable"])) == "null" || json.Unmarshal(fields["retryable"], &item.Retryable) != nil {
				return Receipt{}, invalid
			}
			if json.Unmarshal(fields["reason"], &item.Reason) != nil {
				return Receipt{}, invalid
			}
			receipt.Results = append(receipt.Results, item)
		}
	}
	if err := validate(receipt, batch); err != nil {
		return Receipt{}, err
	}
	sort.Slice(receipt.Results, func(i, j int) bool { return receipt.Results[i].Index < receipt.Results[j].Index })
	return receipt, nil
}

func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("extra JSON document")
	}
	return nil
}

func validate[R any](receipt Receipt, batch []buffer.VersionedSnapshot[R]) error {
	invalid := errors.New("invalid Cloud intake receipt")
	if receipt.Saved < 0 || receipt.Filtered < 0 || receipt.Skipped < 0 || receipt.Saved > math.MaxInt64-receipt.Filtered || receipt.Saved+receipt.Filtered > math.MaxInt64-receipt.Skipped || receipt.Saved+receipt.Filtered+receipt.Skipped != int64(len(batch)) {
		return invalid
	}
	if receipt.Legacy {
		if receipt.Skipped != 0 || len(receipt.Results) != 0 {
			return invalid
		}
		return nil
	}
	if len(receipt.Results) != len(batch) {
		return invalid
	}
	seen := make(map[int]bool, len(batch))
	counts := map[string]int64{"saved": 0, "filtered": 0, "skipped": 0}
	for _, item := range receipt.Results {
		if item.Index < 0 || item.Index >= len(batch) || seen[item.Index] || item.Hash == "" || item.Hash != batch[item.Index].Hash {
			return invalid
		}
		if _, known := counts[item.Status]; !known {
			return invalid
		}
		if item.Status != "skipped" && item.Retryable {
			return invalid
		}
		if item.Status == "skipped" && (item.Reason == nil || *item.Reason == "") {
			return invalid
		}
		seen[item.Index] = true
		counts[item.Status]++
	}
	if counts["saved"] != receipt.Saved || counts["filtered"] != receipt.Filtered || counts["skipped"] != receipt.Skipped {
		return invalid
	}
	return nil
}
