package inventory

import (
	"strings"
	"testing"
	"time"
)

func TestGenerationContainsNoUppercaseHex(t *testing.T) {
	generation := Aggregate("cluster-a", nil, time.Unix(100, 0)).Generation
	if generation != strings.ToLower(generation) {
		t.Fatalf("generation must use lowercase hexadecimal: %q", generation)
	}
}
