package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

func TestUploadReservationAllowsLargeFilesAndCountsConcurrentRequests(t *testing.T) {
	device := ^uint64(0)
	limit, release, err := reserveUpload(device, 12<<30, 3<<30)
	if err != nil || limit != 3<<30 {
		t.Fatalf("3 GiB reservation: limit=%d err=%v", limit, err)
	}
	defer release()
	if _, _, err := reserveUpload(device, 12<<30, 9<<30); !errors.Is(err, ErrUploadSpace) {
		t.Fatalf("concurrent requests exceeded budget: %v", err)
	}
	release()
	release()
	limit, releaseUnknown, err := reserveUpload(device, 12<<30, -1)
	if err != nil || limit != (12<<30)-uploadDiskReserve {
		t.Fatalf("unknown size: limit=%d err=%v", limit, err)
	}
	releaseUnknown()
	if _, _, err := reserveUpload(device, uploadDiskReserve, 1); !errors.Is(err, ErrUploadSpace) {
		t.Fatalf("disk reserve was consumed: %v", err)
	}

	var wg sync.WaitGroup
	releases := make(chan func(), 8)
	for range 8 {
		wg.Go(func() {
			if _, release, err := reserveUpload(device, 1<<30, 128<<20); err == nil {
				releases <- release
			}
		})
	}
	wg.Wait()
	close(releases)
	if len(releases) != 6 {
		t.Fatalf("admitted %d requests, want 6", len(releases))
	}
	for release := range releases {
		release()
	}
}

func TestPrepareUploadReusesSeekableFileAtCurrentOffset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("prefixbody"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	unused := filepath.Join(t.TempDir(), "must-not-be-created")
	reader, hash, cleanup, err := prepareUpload(context.Background(), file, 4, unused)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	wantHash := sha256.Sum256([]byte("body"))
	if reader != file || hash != hex.EncodeToString(wantHash[:]) {
		t.Fatal("seekable file was copied or hashed incorrectly")
	}
	if _, err := os.Stat(unused); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected staging directory: %v", err)
	}
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != "body" {
		t.Fatalf("reader was not rewound: %q %v", body, err)
	}
	cleanup()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("caller-owned file was closed: %v", err)
	}
}

func TestPrepareUploadStagesStreamsAndRemovesFailedCopies(t *testing.T) {
	dir := t.TempDir()
	reader, hash, cleanup, err := prepareUpload(context.Background(), struct{ io.Reader }{strings.NewReader("body")}, 4, dir)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256([]byte("body"))
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != "body" || hash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("staged stream: %q %s %v", body, hash, err)
	}
	cleanup()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		body string
		size int64
		err  error
	}{
		{"empty", context.Background(), "", 0, domain.ErrValidation},
		{"short body", context.Background(), "body", 8, domain.ErrValidation},
		{"long body", context.Background(), "body", 2, domain.ErrValidation},
		{"cancelled", cancelled, "body", 4, context.Canceled},
		{"insufficient space", context.Background(), "body", 1 << 62, ErrUploadSpace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := prepareUpload(tc.ctx, struct{ io.Reader }{strings.NewReader(tc.body)}, tc.size, dir)
			if !errors.Is(err, tc.err) {
				t.Fatalf("got %v, want %v", err, tc.err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("temporary files leaked: %v %v", entries, err)
			}
		})
	}
}

type cleanupBlobs struct {
	domain.BlobStore
	err          error
	ids          []int
	deadline     bool
	beforeDelete func()
}

func (b *cleanupBlobs) DeleteMessages(ctx context.Context, _ int64, ids []int) error {
	if b.beforeDelete != nil {
		b.beforeDelete()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	b.deadline = ok && time.Until(deadline) <= 30*time.Second
	b.ids = append([]int(nil), ids...)
	return b.err
}

type cleanupParts struct {
	domain.PartRepository
	stored     []domain.FilePart
	deleted    bool
	persistErr error
	deleteErr  error
}

func (p *cleanupParts) ReplaceParts(ctx context.Context, _ string, uploaded []domain.FilePart) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.stored = append([]domain.FilePart(nil), uploaded...)
	return p.persistErr
}

func (p *cleanupParts) DeleteByFile(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.deleted = true
	return p.deleteErr
}

type cleanupNodes struct {
	domain.NodeRepository
	deleted bool
}

func (n *cleanupNodes) SoftDelete(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n.deleted = true
	return nil
}

func TestCleanupUploadSurvivesDisconnectAndRetainsFailedRemoteIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	uploaded := []domain.FilePart{{MessageID: 41, ChannelID: 7}, {MessageID: 42, ChannelID: 7}}
	remoteErr, metadataErr := errors.New("Telegram unavailable"), errors.New("database unavailable")
	for _, tc := range []struct {
		name       string
		remoteErr  error
		persistErr error
		deleteErr  error
	}{
		{"success after cancellation", nil, nil, nil},
		{"remote deletion failed", remoteErr, nil, nil},
		{"remote and metadata failed", remoteErr, metadataErr, nil},
		{"metadata deletion failed", nil, nil, metadataErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blobs := &cleanupBlobs{err: tc.remoteErr}
			parts := &cleanupParts{persistErr: tc.persistErr, deleteErr: tc.deleteErr}
			blobs.beforeDelete = func() {
				if !reflect.DeepEqual(parts.stored, uploaded) {
					t.Fatal("recovery mappings were not saved before remote cleanup")
				}
			}
			nodes := &cleanupNodes{}
			err := cleanupUpload(ctx, blobs, parts, nodes, "file", uploaded)
			for _, want := range []error{tc.remoteErr, tc.persistErr, tc.deleteErr} {
				if want != nil && !errors.Is(err, want) {
					t.Fatalf("lost cleanup error %v: %v", want, err)
				}
			}
			if !blobs.deadline || !reflect.DeepEqual(blobs.ids, []int{41, 42}) {
				t.Fatal("cleanup inherited cancellation, lost IDs, or lacked a deadline")
			}
			if tc.remoteErr != nil {
				if !reflect.DeepEqual(parts.stored, uploaded) || parts.deleted || nodes.deleted {
					t.Fatal("failed remote cleanup lost recovery mappings")
				}
			} else if tc.deleteErr != nil {
				if nodes.deleted {
					t.Fatal("metadata failure was ignored")
				}
			} else if err != nil || !parts.deleted || !nodes.deleted {
				t.Fatalf("cleanup incomplete: %v", err)
			}
		})
	}
}
