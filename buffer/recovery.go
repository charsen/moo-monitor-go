package buffer

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gitee.com/charsen/moo-go-monitor/records"
	"go.yaml.in/yaml/v3"
)

type typedIntent[R any] struct {
	Version      int
	Hash, Target string
	Record       *R
	Overflow     int64
}

type intent = typedIntent[records.Record]

type diskIntent struct {
	Version  int       `yaml:"version"`
	Hash     string    `yaml:"hash"`
	Target   string    `yaml:"target"`
	Record   yaml.Node `yaml:"record"`
	Overflow int64     `yaml:"overflow"`
}

func (b *store[R, S, V]) writeIntent(change typedIntent[R]) error {
	disk := diskIntent{Version: change.Version, Hash: change.Hash, Target: change.Target, Overflow: change.Overflow}
	if change.Record != nil {
		data, err := b.builder.Encode(*change.Record)
		if err != nil {
			return err
		}
		var node yaml.Node
		if err = yaml.Unmarshal(data, &node); err != nil {
			return errors.New("cannot prepare buffer intent")
		}
		disk.Record = *node.Content[0]
	}
	data, err := yaml.Marshal(disk)
	if err != nil {
		return errors.New("cannot encode buffer intent")
	}
	return b.atomicWrite("intent.yaml", data)
}

// recover replays one fixed final image; it never invokes Build, Refresh or increments.
func (b *store[R, S, V]) recover() error {
	data, exists, err := b.read("intent.yaml")
	if err != nil || !exists {
		return err
	}
	// Presence matters: a missing scalar must not silently become zero and clear overflow.
	var document yaml.Node
	if err = yaml.Unmarshal(data, &document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return errors.New("invalid buffer intent document")
	}
	for i := 0; i < len(document.Content[0].Content); i += 2 {
		key, value := document.Content[0].Content[i], document.Content[0].Content[i+1]
		if key.Kind == yaml.ScalarNode && key.Value == "version" && value.Kind == yaml.ScalarNode && value.Tag == "!!int" {
			var version int
			if value.Decode(&version) == nil && version == 2 {
				return b.recoverDiagnostic(data)
			}
		}
	}
	required := map[string]bool{"version": false, "hash": false, "target": false, "record": false, "overflow": false}
	for i := 0; i < len(document.Content[0].Content); i += 2 {
		key := document.Content[0].Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return errors.New("invalid buffer intent key")
		}
		present, known := required[key.Value]
		if !known || present {
			return errors.New("unknown or duplicate buffer intent key")
		}
		value := document.Content[0].Content[i+1]
		switch key.Value {
		case "version", "overflow":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!int" {
				return errors.New("invalid buffer intent integer")
			}
		case "hash", "target":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return errors.New("invalid buffer intent string")
			}
		case "record":
			if !(value.Kind == yaml.MappingNode && value.Tag == "!!map") && !(value.Kind == yaml.ScalarNode && value.Tag == "!!null") {
				return errors.New("invalid buffer intent record node")
			}
		}
		required[key.Value] = true
	}
	for _, present := range required {
		if !present {
			return errors.New("incomplete buffer intent; original retained")
		}
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var disk diskIntent
	if err = decoder.Decode(&disk); err != nil {
		return errors.New("invalid buffer intent; original retained")
	}
	var tail yaml.Node
	if err = decoder.Decode(&tail); err != io.EOF {
		return errors.New("buffer intent must contain one document")
	}
	if disk.Version != 1 || !validHash.MatchString(disk.Hash) || !knownBucket(disk.Target) || disk.Overflow < 0 {
		return errors.New("invalid buffer intent identity")
	}
	// Never overwrite malformed current files merely because an intent exists.
	existing, bucket, err := b.find(disk.Hash)
	if err != nil {
		return err
	}
	if _, err = b.readOverflow(disk.Hash); err != nil {
		return err
	}
	if disk.Record.Kind != 0 && disk.Record.Tag != "!!null" {
		encoded, err := yaml.Marshal(&disk.Record)
		if err != nil {
			return errors.New("invalid buffer intent record")
		}
		final, err := b.builder.Decode(encoded, disk.Hash)
		if err != nil {
			return errors.New("invalid buffer intent record")
		}
		state := b.builder.State(*final)
		if !knownBucket(state.Status) || state.Status != disk.Target {
			return errors.New("invalid buffer intent status")
		}
		if err = b.writeRecord(disk.Target, *final); err != nil {
			return fmt.Errorf("buffer recovery incomplete; intent retained: %w", err)
		}
		for _, old := range buckets {
			if old == disk.Target {
				continue
			}
			if err = b.remove(old + "/" + disk.Hash + ".yaml"); err != nil {
				return fmt.Errorf("buffer recovery incomplete; intent retained: %w", err)
			}
		}
	} else if existing == nil || bucket != "open" || disk.Target != "open" || disk.Overflow == 0 {
		return errors.New("frozen buffer intent lacks open record")
	}
	if err = b.writeOverflow(disk.Hash, disk.Overflow); err != nil {
		return fmt.Errorf("buffer recovery incomplete; intent retained: %w", err)
	}
	if err = b.remove("intent.yaml"); err != nil {
		return fmt.Errorf("buffer recovery incomplete; check intent before retry: %w", err)
	}
	return nil
}
