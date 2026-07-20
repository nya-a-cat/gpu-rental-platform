package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://control-plane:secret@postgres/control-plane")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("CONTROL_PLANE_ADDR", "")
	t.Setenv("AGENT_HEARTBEAT_INTERVAL", "")
	t.Setenv("AGENT_DEGRADED_AFTER", "")
	t.Setenv("AGENT_OFFLINE_AFTER", "")

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
	if cfg.AgentHealthPolicy.HeartbeatInterval != 15*time.Second ||
		cfg.AgentHealthPolicy.DegradedAfter != 45*time.Second ||
		cfg.AgentHealthPolicy.OfflineAfter != 90*time.Second {
		t.Fatalf("agent health policy = %#v, want 15s/45s/90s", cfg.AgentHealthPolicy)
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

func TestLoadConfiguresAgentHealthPolicy(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://control-plane:secret@postgres/control-plane")
	t.Setenv("AGENT_HEARTBEAT_INTERVAL", "10s")
	t.Setenv("AGENT_DEGRADED_AFTER", "40s")
	t.Setenv("AGENT_OFFLINE_AFTER", "2m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AgentHealthPolicy.HeartbeatInterval != 10*time.Second ||
		cfg.AgentHealthPolicy.DegradedAfter != 40*time.Second ||
		cfg.AgentHealthPolicy.OfflineAfter != 2*time.Minute {
		t.Fatalf("agent health policy = %#v", cfg.AgentHealthPolicy)
	}
}

func TestLoadRejectsInvalidAgentHealthPolicyOrder(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://control-plane:secret@postgres/control-plane")
	t.Setenv("AGENT_HEARTBEAT_INTERVAL", "15s")
	t.Setenv("AGENT_DEGRADED_AFTER", "10s")
	t.Setenv("AGENT_OFFLINE_AFTER", "90s")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want agent health policy validation error")
	}
}

func TestLoadConfiguresBreakGlassAdmin(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://control-plane:secret@postgres/control-plane")
	t.Setenv("BREAK_GLASS_ADMIN_TOKEN", "0123456789abcdef0123456789abcdef")
	t.Setenv("BREAK_GLASS_ADMIN_SUBJECT", "emergency-admin")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BreakGlassAdminToken != "0123456789abcdef0123456789abcdef" || cfg.BreakGlassAdminSubject != "emergency-admin" {
		t.Fatalf("break-glass config = %q/%q", cfg.BreakGlassAdminSubject, cfg.BreakGlassAdminToken)
	}
}

func TestLoadRejectsShortBreakGlassToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://control-plane:secret@postgres/control-plane")
	t.Setenv("BREAK_GLASS_ADMIN_TOKEN", "short")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want break-glass token validation error")
	}
}
