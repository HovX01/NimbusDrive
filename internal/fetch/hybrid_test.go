package fetch

import "testing"

func TestVidbeeFormat(t *testing.T) {
	if got := vidbeeFormat(720, "video"); got == "" {
		t.Fatal("expected video format")
	}
	if got := vidbeeFormat(0, "audio"); got != "bestaudio/best" {
		t.Fatalf("audio format=%q", got)
	}
}

func TestVidbeeAPIConfigured(t *testing.T) {
	t.Setenv("NIMBUS_VIDBEE_API", "")
	if vidbeeAPIConfigured() {
		t.Fatal("expected false")
	}
	t.Setenv("NIMBUS_VIDBEE_API", "http://localhost:3100")
	if !vidbeeAPIConfigured() {
		t.Fatal("expected true")
	}
}
