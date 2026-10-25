package telegram

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestChunkObjectNameIsOpaque(t *testing.T) {
	seen := make(map[string]struct{}, 1024)
	for i := 0; i < 1024; i++ {
		name, err := chunkObjectName()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(name, ".bin") {
			t.Fatalf("missing generic suffix: %q", name)
		}
		raw := strings.TrimSuffix(name, ".bin")
		if len(raw) != 32 {
			t.Fatalf("unexpected name length: %q", name)
		}
		if _, err := hex.DecodeString(raw); err != nil {
			t.Fatalf("name is not hex: %q", name)
		}
		if strings.Contains(strings.ToLower(name), "nimbus") || strings.Contains(strings.ToLower(name), "part") {
			t.Fatalf("name leaks storage identity: %q", name)
		}
		if _, dup := seen[name]; dup {
			t.Fatalf("duplicate chunk name: %q", name)
		}
		seen[name] = struct{}{}
	}
}
