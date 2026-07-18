package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	config, err := loadConfig(func(string) string { return "" })
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if config.URL != defaultHealthcheckURL {
		t.Fatalf("URL = %q, want %q", config.URL, defaultHealthcheckURL)
	}
	if config.Timeout != 2*time.Second {
		t.Fatalf("Timeout = %s, want 2s", config.Timeout)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	values := map[string]string{
		"HEALTHCHECK_URL":     "https://control-plane.example/health/ready",
		"HEALTHCHECK_TIMEOUT": "750ms",
	}
	config, err := loadConfig(func(name string) string { return values[name] })
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if config.URL != values["HEALTHCHECK_URL"] || config.Timeout != 750*time.Millisecond {
		t.Fatalf("config = %#v", config)
	}
}

func TestLoadConfigRejectsInvalidTimeout(t *testing.T) {
	_, err := loadConfig(func(name string) string {
		if name == "HEALTHCHECK_TIMEOUT" {
			return "0s"
		}
		return ""
	})
	if err == nil {
		t.Fatal("loadConfig() error = nil, want timeout validation error")
	}
}

func TestCheckAccepts2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := check(context.Background(), server.Client(), server.URL); err != nil {
		t.Fatalf("check() error = %v", err)
	}
}

func TestCheckRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if err := check(context.Background(), server.Client(), server.URL); err == nil {
		t.Fatal("check() error = nil, want non-2xx error")
	}
}
