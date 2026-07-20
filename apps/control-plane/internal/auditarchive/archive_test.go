package auditarchive

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type staticSource struct {
	data string
	rows int64
}

func (source staticSource) WriteJSONL(_ context.Context, _, _ time.Time, destination io.Writer) (int64, error) {
	_, err := io.WriteString(destination, source.data)
	return source.rows, err
}

type memoryStore struct {
	objects map[string]storedObject
	puts    int
}

type storedObject struct {
	data    []byte
	options PutOptions
}

func (store *memoryStore) Stat(_ context.Context, bucket, object string) (ObjectInfo, error) {
	stored, ok := store.objects[bucket+"/"+object]
	if !ok {
		return ObjectInfo{}, ErrObjectNotFound
	}
	return ObjectInfo{Size: int64(len(stored.data)), Metadata: stored.options.Metadata}, nil
}

func (store *memoryStore) Put(_ context.Context, bucket, object string, reader io.Reader, _ int64, options PutOptions) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	store.puts++
	store.objects[bucket+"/"+object] = storedObject{data: data, options: options}
	return nil
}

func TestArchiveUploadsLockedDeterministicObject(t *testing.T) {
	now := time.Date(2026, time.July, 20, 3, 0, 0, 0, time.UTC)
	store := &memoryStore{objects: map[string]storedObject{}}
	result, err := Archive(context.Background(), Config{
		Bucket:        "audit",
		Prefix:        "platform",
		MonthStart:    time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		RetentionMode: "COMPLIANCE",
		RetentionDays: 365,
		TempDir:       t.TempDir(),
	}, staticSource{data: "{\"id\":\"one\"}\n", rows: 1}, store, now)
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if result.Status != "uploaded" {
		t.Fatalf("status = %q, want uploaded", result.Status)
	}
	if result.ObjectKey != "platform/year=2026/month=06/audit-events-2026-06.jsonl" {
		t.Fatalf("object key = %q", result.ObjectKey)
	}
	if result.Rows != 1 || result.Bytes != int64(len("{\"id\":\"one\"}\n")) {
		t.Fatalf("rows/bytes = %d/%d", result.Rows, result.Bytes)
	}
	stored := store.objects["audit/"+result.ObjectKey]
	if string(stored.data) != "{\"id\":\"one\"}\n" {
		t.Fatalf("stored data = %q", stored.data)
	}
	if stored.options.RetentionMode != "COMPLIANCE" || !stored.options.RetentionUntil.Equal(now.AddDate(0, 0, 365)) {
		t.Fatalf("retention = %s/%s", stored.options.RetentionMode, stored.options.RetentionUntil)
	}
	if stored.options.Metadata["sha256"] != result.SHA256 || stored.options.Metadata["row-count"] != "1" {
		t.Fatalf("metadata = %#v", stored.options.Metadata)
	}
}

func TestArchiveIsIdempotentForMatchingObject(t *testing.T) {
	now := time.Date(2026, time.July, 20, 3, 0, 0, 0, time.UTC)
	store := &memoryStore{objects: map[string]storedObject{}}
	cfg := Config{
		Bucket:        "audit",
		Prefix:        "platform",
		MonthStart:    time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		RetentionMode: "GOVERNANCE",
		RetentionDays: 30,
		TempDir:       t.TempDir(),
	}
	source := staticSource{data: "{\"id\":\"one\"}\n", rows: 1}
	if _, err := Archive(context.Background(), cfg, source, store, now); err != nil {
		t.Fatalf("first Archive() error = %v", err)
	}
	result, err := Archive(context.Background(), cfg, source, store, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("second Archive() error = %v", err)
	}
	if result.Status != "existing" || store.puts != 1 {
		t.Fatalf("status/puts = %q/%d", result.Status, store.puts)
	}
}

func TestArchiveRejectsConflictingObject(t *testing.T) {
	now := time.Date(2026, time.July, 20, 3, 0, 0, 0, time.UTC)
	store := &memoryStore{objects: map[string]storedObject{}}
	cfg := Config{
		Bucket:        "audit",
		Prefix:        "platform",
		MonthStart:    time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		RetentionMode: "GOVERNANCE",
		RetentionDays: 30,
		TempDir:       t.TempDir(),
	}
	if _, err := Archive(context.Background(), cfg, staticSource{data: "first\n", rows: 1}, store, now); err != nil {
		t.Fatalf("first Archive() error = %v", err)
	}
	_, err := Archive(context.Background(), cfg, staticSource{data: "changed\n", rows: 1}, store, now)
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("conflicting Archive() error = %v", err)
	}
}

func TestArchiveValidatesInputs(t *testing.T) {
	store := &memoryStore{objects: map[string]storedObject{}}
	_, err := Archive(context.Background(), Config{}, staticSource{}, store, time.Now())
	if err == nil {
		t.Fatal("Archive() error = nil")
	}
}
