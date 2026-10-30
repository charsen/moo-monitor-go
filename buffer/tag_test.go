package buffer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestTagSourceBuckets(t *testing.T) {
	for _, bucket := range buckets {
		t.Run(bucket, func(t *testing.T) {
			b := openBuffer(t, privateTemp(t), "", 0, 10)
			hash := mustRecord(t, b, testSnapshot(), testNow)
			if bucket != "open" {
				seedBucket(t, b, hash, bucket)
			}
			before := getRecord(t, b, hash)
			if err := b.writeOverflow(hash, 3); err != nil {
				t.Fatal(err)
			}
			if tagged, err := b.TagSource(context.Background(), hash, "queue_failed", map[string]any{"note": "token=fake"}, testNow.Add(time.Hour)); err != nil || !tagged {
				t.Fatal(tagged, err)
			}
			after := getRecord(t, b, hash)
			if after.Meta["source"] != "queue_failed" || after.Meta["note"] != "token=***" || after.Meta["updated_at"] != iso(testNow.Add(time.Hour)) {
				t.Fatal("tag metadata")
			}
			before.Meta = nil
			after.Meta = nil
			if !reflect.DeepEqual(before, after) {
				t.Fatal("tag changed occurrence fields")
			}
			if value, err := b.readOverflow(hash); err != nil || value != 3 {
				t.Fatal("tag drained overflow")
			}
			path := filepath.Join(b.path, bucket, hash+".yaml")
			first, _ := os.ReadFile(path)
			if _, err := b.TagSource(context.Background(), hash, "queue_failed", map[string]any{"note": "token=fake"}, testNow.Add(2*time.Hour)); err != nil {
				t.Fatal(err)
			}
			second, _ := os.ReadFile(path)
			if !bytes.Equal(first, second) {
				t.Fatal("no-change tag rewrote bytes")
			}
			if count, err := b.Count(context.Background(), "all"); err != nil || count != 1 {
				t.Fatal("tag bucket duplicated", err)
			}
		})
	}
	b := openBuffer(t, privateTemp(t), "", 1, 10)
	hash := mustRecord(t, b, testSnapshot(), testNow)
	before, _ := os.ReadFile(filepath.Join(b.path, "open", hash+".yaml"))
	if tagged, err := b.TagSource(context.Background(), hash, "queue_failed", nil, testNow.Add(time.Hour)); err != nil || !tagged {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(b.path, "open", hash+".yaml"))
	if !bytes.Equal(before, after) {
		t.Fatal("cap tag rewrote frozen record")
	}
}
func TestTagRecovery(t *testing.T) {
	for _, bucket := range buckets {
		for _, stage := range []string{"tag-intent", "tag-target", "tag-old-removed"} {
			t.Run(bucket+"/"+stage, func(t *testing.T) {
				root := privateTemp(t)
				b := openBuffer(t, root, "", 0, 10)
				hash := mustRecord(t, b, testSnapshot(), testNow)
				if bucket != "open" {
					seedBucket(t, b, hash, bucket)
				}
				b.writeOverflow(hash, 7)
				injected := errors.New("fixed tag interruption")
				b.fault = func(point string) error {
					if point == stage {
						return injected
					}
					return nil
				}
				if _, err := b.TagSource(context.Background(), hash, "queue_failed", nil, testNow.Add(time.Hour)); !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, injected) {
					t.Fatal(err)
				}
				b.Close()
				reopened := openBuffer(t, root, "", 0, 10)
				record := getRecord(t, reopened, hash)
				if record.Count != 1 || record.Daily.Count != 1 || record.LastSeen != iso(testNow) || record.Status != bucket || record.Meta["source"] != "queue_failed" {
					t.Fatal("tag recovery wrong image")
				}
				if count, err := reopened.Count(context.Background(), "all"); err != nil || count != 1 {
					t.Fatal("tag recovery duplicate", err)
				}
				if value, err := reopened.readOverflow(hash); err != nil || value != 7 {
					t.Fatal("tag recovery overflow")
				}
			})
		}
	}
}

// A valid status in the wrong target bucket is still a corrupt final image.
func TestTagIntentStatusMismatch(t *testing.T) {
	b := openBuffer(t, privateTemp(t), "", 0, 10)
	hash := mustRecord(t, b, testSnapshot(), testNow)
	record := getRecord(t, b, hash)
	original, _, err := b.read("open/" + hash + ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = b.writeOverflow(hash, 7); err != nil {
		t.Fatal(err)
	}
	record.Status = "deleted"
	if err = b.writeIntent(intent{Version: 1, Hash: hash, Target: "open", Record: record, Overflow: 0}); err != nil {
		t.Fatal(err)
	}
	originalIntent, _, err := b.read("intent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Get(context.Background(), hash); err == nil {
		t.Fatal("mismatched status/target intent accepted")
	}
	retainedIntent, _, _ := b.read("intent.yaml")
	retainedRecord, _, _ := b.read("open/" + hash + ".yaml")
	if !bytes.Equal(originalIntent, retainedIntent) || !bytes.Equal(original, retainedRecord) {
		t.Fatal("corrupt intent or original record overwritten")
	}
	if value, err := b.readOverflow(hash); err != nil || value != 7 {
		t.Fatal("corrupt intent changed overflow", value, err)
	}
	for _, bucket := range []string{"resolved", "deleted"} {
		if _, exists, err := b.read(bucket + "/" + hash + ".yaml"); err != nil || exists {
			t.Fatal("corrupt intent wrote another bucket", err)
		}
	}
}
