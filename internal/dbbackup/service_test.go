package dbbackup

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/vrc/nimbus/internal/domain"
)

type fakeDB struct {
	data []byte
	err  error
}

func (f *fakeDB) SnapshotToFile(_ context.Context, path string) error {
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(path, f.data, 0o600)
}

type fakeBlobs struct {
	mu        sync.Mutex
	name      string
	caption   string
	data      []byte
	deleted   []int
	nextID    int
	uploadErr error
}

func (f *fakeBlobs) EnsureStorageChannel(context.Context) (int64, error) { return 42, nil }

func (f *fakeBlobs) UploadPart(context.Context, int64, int, string, io.Reader, int64) (int, error) {
	return 0, nil
}

func (f *fakeBlobs) UploadDocument(_ context.Context, _ int64, name, caption string, r io.Reader, _ int64) (int, error) {
	if f.uploadErr != nil {
		return 0, f.uploadErr
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.name = name
	f.caption = caption
	f.data = data
	return f.nextID, nil
}

func (f *fakeBlobs) DownloadPart(context.Context, int64, int, io.Writer) error { return nil }

func (f *fakeBlobs) DeleteMessages(_ context.Context, _ int64, ids []int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, ids...)
	return nil
}

func (f *fakeBlobs) lastUpload() (string, string, []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.name, f.caption, append([]byte(nil), f.data...)
}

func TestRunOnceUploadsNamedBackup(t *testing.T) {
	dir := t.TempDir()
	blobs := &fakeBlobs{}
	svc := NewService(dir, &fakeDB{data: []byte("sqlite snapshot")}, blobs, "nimbus-db-backup", 0)

	entry, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	name, caption, data := blobs.lastUpload()
	if name != entry.Name {
		t.Fatalf("entry name %q != uploaded name %q", entry.Name, name)
	}
	if !strings.HasPrefix(name, "nimbus-db-backup-") || !strings.HasSuffix(name, ".sqlite") {
		t.Fatalf("backup name is not easy to find: %q", name)
	}
	if !strings.Contains(caption, "Nimbus database backup") || !strings.Contains(caption, "sha256:") {
		t.Fatalf("caption does not identify the backup: %q", caption)
	}
	if string(data) != "sqlite snapshot" {
		t.Fatalf("uploaded data mismatch: %q", data)
	}
	if entry.MessageID != 1 || entry.ChannelID != 42 {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.Size != int64(len("sqlite snapshot")) {
		t.Fatalf("unexpected size: %d", entry.Size)
	}
	if len(entry.SHA256) != 64 {
		t.Fatalf("unexpected sha256: %q", entry.SHA256)
	}

	indexed, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(indexed) != 1 || indexed[0].Name != entry.Name {
		t.Fatalf("index mismatch: %+v", indexed)
	}
}

func TestRunOnceNamesAreUnique(t *testing.T) {
	dir := t.TempDir()
	blobs := &fakeBlobs{}
	svc := NewService(dir, &fakeDB{data: []byte("db")}, blobs, "nimbus-db-backup", 0)

	first, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Name == second.Name {
		t.Fatalf("duplicate backup name: %q", first.Name)
	}
}

func TestRunOncePrunesOldBackups(t *testing.T) {
	dir := t.TempDir()
	blobs := &fakeBlobs{}
	svc := NewService(dir, &fakeDB{data: []byte("db")}, blobs, "nimbus-db-backup", 1)

	first, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexed, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(indexed) != 1 {
		t.Fatalf("keep=1 left %d entries", len(indexed))
	}
	blobs.mu.Lock()
	deleted := append([]int(nil), blobs.deleted...)
	blobs.mu.Unlock()
	if len(deleted) != 1 || deleted[0] != first.MessageID {
		t.Fatalf("old backup not pruned: %v", deleted)
	}
}

func TestRunOnceRejectsUnconfigured(t *testing.T) {
	svc := NewService(t.TempDir(), nil, nil, "", 0)
	if _, err := svc.RunOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected not configured, got %v", err)
	}
}

func TestServiceImplementsUsableBlobShape(t *testing.T) {
	var _ domain.BlobStore = (*fakeBlobs)(nil)
}
