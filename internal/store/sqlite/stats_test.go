package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

func TestStorageStats(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st, err := Open("sqlite://" + filepath.Join(dir, "stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	mk := func(name, mime string, size int64, channel int64) {
		t.Helper()
		n := domain.Node{
			ParentID:  &root.ID,
			Name:      name,
			Type:      domain.NodeFile,
			MimeType:  mime,
			Size:      size,
			Status:    domain.StatusReady,
			ChannelID: channel,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if _, err := st.Create(ctx, n); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	mk("a.mp4", "video/mp4", 1000, 111)
	mk("b.mp4", "video/mp4", 500, 111)
	mk("c.png", "image/png", 200, 222)

	// A deleted file must not count toward live totals.
	trashed := domain.Node{
		ParentID: &root.ID, Name: "old.bin", Type: domain.NodeFile,
		MimeType: "application/octet-stream", Size: 9999,
		Status: domain.StatusReady, CreatedAt: now, UpdatedAt: now,
	}
	created, err := st.Create(ctx, trashed)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SoftDelete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	stats, err := st.StorageStats(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}

	if stats.TotalBytes != 1700 {
		t.Errorf("total_bytes = %d, want 1700", stats.TotalBytes)
	}
	if stats.TotalFiles != 3 {
		t.Errorf("total_files = %d, want 3", stats.TotalFiles)
	}
	if stats.TrashBytes != 9999 || stats.TrashFiles != 1 {
		t.Errorf("trash = %d bytes / %d files, want 9999 / 1", stats.TrashBytes, stats.TrashFiles)
	}

	byKind := map[string]domain.StorageKindStats{}
	for _, k := range stats.ByKind {
		byKind[k.Kind] = k
	}
	if v := byKind["video"]; v.Bytes != 1500 || v.Count != 2 {
		t.Errorf("video = %+v, want 1500 bytes / 2", v)
	}
	if v := byKind["image"]; v.Bytes != 200 || v.Count != 1 {
		t.Errorf("image = %+v, want 200 bytes / 1", v)
	}

	byChan := map[int64]domain.StorageBucketStats{}
	for _, c := range stats.ByChannel {
		byChan[c.ChannelID] = c
	}
	if c := byChan[111]; c.Bytes != 1500 || c.Count != 2 {
		t.Errorf("channel 111 = %+v, want 1500 / 2", c)
	}
	if c := byChan[222]; c.Bytes != 200 || c.Count != 1 {
		t.Errorf("channel 222 = %+v, want 200 / 1", c)
	}

	if len(stats.Daily) == 0 {
		t.Error("expected at least one daily point")
	}
}
