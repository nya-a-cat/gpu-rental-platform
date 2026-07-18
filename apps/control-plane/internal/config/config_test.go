package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://control-plane:secret@postgres/control-plane")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("CONTROL_PLANE_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Fatalf("ShutdownTimeout = %s, want 15s", cfg.ShutdownTimeout)
	}
	if cfg.MaxOpenConns != 20 || cfg.MaxIdleConns != 5 {
		t.Fatalf("pool = %d/%d, want 20/5", cfg.MaxOpenConns, cfg.MaxIdleConns)
	}
	if cfg.MigrationTimeout != 5*time.Minute || cfg.MigrationLockTimeout != 30*time.Second || cfg.MigrationStatementTimeout != 2*time.Minute {
		t.Fatalf(
			"migration timeouts = %s/%s/%s, want 5m/30s/2m",
			cfg.MigrationTimeout,
			cfg.MigrationLockTimeout,
			cfg.MigrationStatementTimeout,
		)
	}
}

func TestLoadPrefersHTTPAddr(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://control-plane:secret@postgres/control-plane")
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	t.Setenv("CONTROL_PLANE_ADDR", ":7070")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:9090" {
		t.Fatalf("HTTPAddr = %q, want 127.0.0.1:9090", cfg.HTTPAddr)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want DATABASE_URL validation error")
	}
}

func TestLoadRejectsSubMillisecondMigrationTimeout(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://control-plane:secret@postgres/control-plane")
	t.Setenv("MIGRATION_LOCK_TIMEOUT", "500us")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want MIGRATION_LOCK_TIMEOUT validation error")
	}
}

func TestLoadRejectsInvalidPool(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://control-plane:secret@postgres/control-plane")
	t.Setenv("DB_MAX_OPEN_CONNS", "2")
	t.Setenv("DB_MAX_IDLE_CONNS", "3")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want pool validation error")
	}
}
