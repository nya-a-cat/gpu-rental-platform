package authn

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBreakGlassAuthenticator(t *testing.T) {
	authenticator, err := NewBreakGlassAuthenticator(
		"break-glass-admin",
		"0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatalf("NewBreakGlassAuthenticator() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", nil)
	request.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef")
	principal, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.ID != "break-glass-admin" || !principal.SystemAdmin {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestBreakGlassAuthenticatorRejectsInvalidCredentials(t *testing.T) {
	authenticator, err := NewBreakGlassAuthenticator(
		"break-glass-admin",
		"0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatalf("NewBreakGlassAuthenticator() error = %v", err)
	}
	for _, authorization := range []string{"", "Basic token", "Bearer wrong"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/id", nil)
		request.Header.Set("Authorization", authorization)
		if _, err := authenticator.Authenticate(request); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("Authenticate(%q) error = %v, want ErrUnauthenticated", authorization, err)
		}
	}
}

func TestBreakGlassAuthenticatorRequiresStrongConfiguration(t *testing.T) {
	if _, err := NewBreakGlassAuthenticator("", "0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("empty subject error = nil")
	}
	if _, err := NewBreakGlassAuthenticator("admin", "short"); err == nil {
		t.Fatal("short token error = nil")
	}
}
