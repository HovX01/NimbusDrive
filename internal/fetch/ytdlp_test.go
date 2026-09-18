package fetch

import "testing"

func TestYtDLPFormat(t *testing.T) {
	if got := ytDLPFormat(720); got != "bv*[height<=720]+ba/b[height<=720]/best" {
		t.Fatalf("format=%q", got)
	}
	if got := ytDLPFormat(0); got != "bv*+ba/best" {
		t.Fatalf("format=%q", got)
	}
}

func TestYtDLPMaxDownloads(t *testing.T) {
	t.Setenv("NIMBUS_YTDLP_MAX_DOWNLOADS", "")
	if got := ytDLPMaxDownloads(); got != 50 {
		t.Fatalf("max=%d", got)
	}
	t.Setenv("NIMBUS_YTDLP_MAX_DOWNLOADS", "7")
	if got := ytDLPMaxDownloads(); got != 7 {
		t.Fatalf("max=%d", got)
	}
}

func TestYtDLPAllowPlaylist(t *testing.T) {
	t.Setenv("NIMBUS_YTDLP_ALLOW_PLAYLIST", "")
	if ytDLPAllowPlaylist() {
		t.Fatal("expected playlist disabled")
	}
	t.Setenv("NIMBUS_YTDLP_ALLOW_PLAYLIST", "true")
	if !ytDLPAllowPlaylist() {
		t.Fatal("expected playlist enabled")
	}
}
