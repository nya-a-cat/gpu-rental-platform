package auditarchive

import (
	"testing"
	"time"
)

func TestLoadRuntimeConfigDefaultsToLatestAgedCompleteMonth(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":                "postgres://control-plane:secret@postgres/control-plane",
		"AUDIT_ARCHIVE_S3_ENDPOINT":   "https://objects.example.test",
		"AUDIT_ARCHIVE_S3_ACCESS_KEY": "access",
		"AUDIT_ARCHIVE_S3_SECRET_KEY": "secret",
		"AUDIT_ARCHIVE_S3_BUCKET":     "audit",
	}
	cfg, err := LoadRuntimeConfig(func(key string) string { return values[key] }, time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	if got := cfg.Archive.MonthStart.Format("2006-01"); got != "2026-06" {
		t.Fatalf("month = %s, want 2026-06", got)
	}
	if cfg.Archive.RetentionMode != "GOVERNANCE" || cfg.Archive.RetentionDays != 365 {
		t.Fatalf("retention = %s/%d", cfg.Archive.RetentionMode, cfg.Archive.RetentionDays)
	}
	if cfg.CommandTimeout != 10*time.Minute || cfg.Region != "us-east-1" {
		t.Fatalf("timeout/region = %s/%s", cfg.CommandTimeout, cfg.Region)
	}
}

func TestLoadRuntimeConfigSupportsExplicitMonth(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":                 "postgres://control-plane:secret@postgres/control-plane",
		"AUDIT_ARCHIVE_S3_ENDPOINT":    "http://minio:9000",
		"AUDIT_ARCHIVE_S3_ACCESS_KEY":  "access",
		"AUDIT_ARCHIVE_S3_SECRET_KEY":  "secret",
		"AUDIT_ARCHIVE_S3_BUCKET":      "audit",
		"AUDIT_ARCHIVE_MONTH":          "2026-07",
		"AUDIT_ARCHIVE_RETENTION_MODE": "compliance",
		"AUDIT_ARCHIVE_RETENTION_DAYS": "730",
	}
	cfg, err := LoadRuntimeConfig(func(key string) string { return values[key] }, time.Now())
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	if got := cfg.Archive.MonthStart.Format("2006-01"); got != "2026-07" {
		t.Fatalf("month = %s, want 2026-07", got)
	}
	if cfg.Archive.RetentionMode != "COMPLIANCE" || cfg.Archive.RetentionDays != 730 {
		t.Fatalf("retention = %s/%d", cfg.Archive.RetentionMode, cfg.Archive.RetentionDays)
	}
}

func TestLoadRuntimeConfigRejectsInvalidEndpoint(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":                "postgres://control-plane:secret@postgres/control-plane",
		"AUDIT_ARCHIVE_S3_ENDPOINT":   "minio:9000",
		"AUDIT_ARCHIVE_S3_ACCESS_KEY": "access",
		"AUDIT_ARCHIVE_S3_SECRET_KEY": "secret",
		"AUDIT_ARCHIVE_S3_BUCKET":     "audit",
	}
	if _, err := LoadRuntimeConfig(func(key string) string { return values[key] }, time.Now()); err == nil {
		t.Fatal("LoadRuntimeConfig() error = nil")
	}
}
