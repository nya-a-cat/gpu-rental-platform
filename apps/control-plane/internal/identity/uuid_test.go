package identity

import "testing"

func TestNewUUID(t *testing.T) {
	value, err := NewUUID()
	if err != nil {
		t.Fatalf("NewUUID() error = %v", err)
	}
	if !IsUUID(value) {
		t.Fatalf("NewUUID() = %q, want UUID", value)
	}
	if value[14] != '4' {
		t.Fatalf("NewUUID() version = %q, want 4", value[14])
	}
}

func TestIsUUIDRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{"", "123", "00000000-0000-0000-0000-00000000000z"} {
		if IsUUID(value) {
			t.Fatalf("IsUUID(%q) = true, want false", value)
		}
	}
}
