package fetch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSniffMediaExt(t *testing.T) {
	jpg := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0, 0, 0, 0, 0, 0, 0}
	if got := sniffMediaExt(jpg); got != ".jpg" {
		t.Fatalf("jpg: got %q", got)
	}
	mp4 := make([]byte, 12)
	copy(mp4[4:], []byte("ftypisom"))
	if got := sniffMediaExt(mp4); got != ".mp4" {
		t.Fatalf("mp4: got %q", got)
	}
	heic := make([]byte, 12)
	copy(heic[4:], []byte("ftypheic"))
	if got := sniffMediaExt(heic); got != ".heic" {
		t.Fatalf("heic: got %q", got)
	}
}

func TestMaybeRenameByExtLivePhoto(t *testing.T) {
	name, dest := maybeRenameByExt("cobalt_photo_1.jpg", "/tmp", ".mp4")
	if filepath.Ext(name) != ".mp4" {
		t.Fatalf("ext: %s", name)
	}
	if !strings.Contains(name, "live") {
		t.Fatalf("expected live in name, got %s", name)
	}
	if filepath.Base(dest) != name {
		t.Fatalf("dest base %s != %s", filepath.Base(dest), name)
	}
}
