package inventory

import (
	"encoding/hex"
	"testing"
	"time"
)

func TestGenerationUsesLowercaseSHA256Hex(t *testing.T) {
	snapshot := Aggregate("cluster-a", nil, time.Unix(100, 0))
	if len(snapshot.Generation) != 64 {
		t.Fatalf("expected 64-character generation, got %d: %q", len(snapshot.Generation), snapshot.Generation)
	}
	decoded, err := hex.DecodeString(snapshot.Generation)
	if err != nil {
		t.Fatalf("generation is not lowercase hexadecimal: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("expected 32-byte SHA-256 digest, got %d bytes", len(decoded))
	}
}
