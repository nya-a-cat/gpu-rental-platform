package auditarchive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

var ErrObjectNotFound = errors.New("audit archive object not found")

type Source interface {
	WriteJSONL(context.Context, time.Time, time.Time, io.Writer) (int64, error)
}

type ObjectInfo struct {
	Size     int64
	Metadata map[string]string
}

type PutOptions struct {
	ContentType    string
	Metadata       map[string]string
	RetentionMode  string
	RetentionUntil time.Time
}

type Store interface {
	Stat(context.Context, string, string) (ObjectInfo, error)
	Put(context.Context, string, string, io.Reader, int64, PutOptions) error
}

type Config struct {
	Bucket        string
	Prefix        string
	MonthStart    time.Time
	RetentionMode string
	RetentionDays int
	TempDir       string
}

type Result struct {
	Status         string
	ObjectKey      string
	Rows           int64
	Bytes          int64
	SHA256         string
	PeriodStart    time.Time
	PeriodEnd      time.Time
	RetentionUntil time.Time
}

func Archive(ctx context.Context, cfg Config, source Source, store Store, now time.Time) (Result, error) {
	if source == nil {
		return Result{}, errors.New("audit archive source is required")
	}
	if store == nil {
		return Result{}, errors.New("audit archive object store is required")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return Result{}, errors.New("audit archive bucket is required")
	}
	if cfg.MonthStart.IsZero() || cfg.MonthStart.Day() != 1 {
		return Result{}, errors.New("audit archive month start must be the first day of a month")
	}
	if cfg.RetentionMode != "GOVERNANCE" && cfg.RetentionMode != "COMPLIANCE" {
		return Result{}, fmt.Errorf("unsupported audit archive retention mode: %s", cfg.RetentionMode)
	}
	if cfg.RetentionDays < 1 {
		return Result{}, errors.New("audit archive retention days must be greater than zero")
	}

	periodStart := cfg.MonthStart.UTC()
	periodEnd := periodStart.AddDate(0, 1, 0)
	objectKey := path.Join(strings.Trim(cfg.Prefix, "/"),
		fmt.Sprintf("year=%04d", periodStart.Year()),
		fmt.Sprintf("month=%02d", int(periodStart.Month())),
		fmt.Sprintf("audit-events-%s.jsonl", periodStart.Format("2006-01")),
	)

	temporary, err := os.CreateTemp(cfg.TempDir, "gpu-control-plane-audit-*.jsonl")
	if err != nil {
		return Result{}, fmt.Errorf("create audit archive temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	defer temporary.Close()

	digest := sha256.New()
	rows, err := source.WriteJSONL(ctx, periodStart, periodEnd, io.MultiWriter(temporary, digest))
	if err != nil {
		return Result{}, fmt.Errorf("export audit events: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return Result{}, fmt.Errorf("sync audit archive temporary file: %w", err)
	}
	stat, err := temporary.Stat()
	if err != nil {
		return Result{}, fmt.Errorf("stat audit archive temporary file: %w", err)
	}
	sha := hex.EncodeToString(digest.Sum(nil))
	metadata := map[string]string{
		"sha256":       sha,
		"row-count":    strconv.FormatInt(rows, 10),
		"period-start": periodStart.Format(time.RFC3339),
		"period-end":   periodEnd.Format(time.RFC3339),
		"schema":       "gpu-control-plane-audit-v1",
	}
	retentionUntil := now.UTC().AddDate(0, 0, cfg.RetentionDays)

	existing, err := store.Stat(ctx, cfg.Bucket, objectKey)
	if err == nil {
		if err := verifyObject(existing, stat.Size(), metadata); err != nil {
			return Result{}, fmt.Errorf("existing audit archive conflicts with current export: %w", err)
		}
		return Result{
			Status:         "existing",
			ObjectKey:      objectKey,
			Rows:           rows,
			Bytes:          stat.Size(),
			SHA256:         sha,
			PeriodStart:    periodStart,
			PeriodEnd:      periodEnd,
			RetentionUntil: retentionUntil,
		}, nil
	}
	if !errors.Is(err, ErrObjectNotFound) {
		return Result{}, fmt.Errorf("stat audit archive object: %w", err)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return Result{}, fmt.Errorf("rewind audit archive temporary file: %w", err)
	}
	if err := store.Put(ctx, cfg.Bucket, objectKey, temporary, stat.Size(), PutOptions{
		ContentType:    "application/x-ndjson",
		Metadata:       metadata,
		RetentionMode:  cfg.RetentionMode,
		RetentionUntil: retentionUntil,
	}); err != nil {
		return Result{}, fmt.Errorf("upload audit archive object: %w", err)
	}
	uploaded, err := store.Stat(ctx, cfg.Bucket, objectKey)
	if err != nil {
		return Result{}, fmt.Errorf("verify uploaded audit archive object: %w", err)
	}
	if err := verifyObject(uploaded, stat.Size(), metadata); err != nil {
		return Result{}, fmt.Errorf("uploaded audit archive verification failed: %w", err)
	}

	return Result{
		Status:         "uploaded",
		ObjectKey:      objectKey,
		Rows:           rows,
		Bytes:          stat.Size(),
		SHA256:         sha,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		RetentionUntil: retentionUntil,
	}, nil
}

func verifyObject(info ObjectInfo, expectedSize int64, expectedMetadata map[string]string) error {
	if info.Size != expectedSize {
		return fmt.Errorf("size = %d, want %d", info.Size, expectedSize)
	}
	for key, expected := range expectedMetadata {
		actual, ok := metadataValue(info.Metadata, key)
		if !ok {
			return fmt.Errorf("metadata %q is missing", key)
		}
		if actual != expected {
			return fmt.Errorf("metadata %q = %q, want %q", key, actual, expected)
		}
	}
	return nil
}

func metadataValue(metadata map[string]string, wanted string) (string, bool) {
	for key, value := range metadata {
		normalized := strings.TrimPrefix(strings.ToLower(key), "x-amz-meta-")
		if normalized == strings.ToLower(wanted) {
			return value, true
		}
	}
	return "", false
}
