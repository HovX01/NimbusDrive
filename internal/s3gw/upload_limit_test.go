package s3gw

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	minio "github.com/minio/minio-go/v7"
	"github.com/vrc/nimbus/internal/domain"
)

type blockedUploads struct {
	domain.BlobStore
	s3, web chan time.Time
	release chan struct{}
}

func (b *blockedUploads) UploadPart(ctx context.Context, channelID int64, partNo int, name string, r io.Reader, size int64) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	entered := b.s3
	if string(data) == "ww" {
		entered = b.web
	}
	entered <- time.Now()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-b.release:
		return b.BlobStore.UploadPart(ctx, channelID, partNo, name, strings.NewReader(string(data)), size)
	}
}

func TestGatewaySharesS3SlotsWhileWebKeepsWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	svc, ts, _ := newTestServer(t)
	if _, err := CreateBucket(ctx, svc, "backups"); err != nil {
		t.Fatal(err)
	}
	blobs := &blockedUploads{
		BlobStore: svc.Blobs, s3: make(chan time.Time, 6), web: make(chan time.Time, 3), release: make(chan struct{}),
	}
	var once sync.Once
	release := func() { once.Do(func() { close(blobs.release) }) }
	defer release()
	svc.Blobs = blobs
	svc.ChunkSize = 2
	svc.UploadWorkers = 3
	client := minioClient(t, ts)
	done := make(chan error, 7)
	for i := range 6 {
		go func() {
			_, err := client.PutObject(ctx, "backups", fmt.Sprintf("db-%d", i), strings.NewReader("s"), 1, minio.PutObjectOptions{})
			done <- err
		}()
	}
	var previous time.Time
	for range 2 {
		select {
		case at := <-blobs.s3:
			if !previous.IsZero() && at.Sub(previous) < 950*time.Millisecond {
				t.Fatal("simultaneous backups bypassed shared pacing")
			}
			previous = at
		case <-ctx.Done():
			t.Fatal("S3 upload did not reach Telegram")
		}
	}
	select {
	case <-blobs.s3:
		t.Fatal("more than two S3 chunks reached Telegram")
	case <-time.After(100 * time.Millisecond):
	}
	go func() {
		_, err := svc.Upload(ctx, rootID, "web.txt", "text/plain", strings.NewReader("wwwwww"), 6)
		done <- err
	}()
	for range 3 {
		select {
		case <-blobs.web:
		case <-ctx.Done():
			t.Fatal("S3 queue blocked web workers")
		}
	}
	release()
	for range 7 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("uploads did not finish")
		}
	}
	for i := range 6 {
		object, err := client.GetObject(ctx, "backups", fmt.Sprintf("db-%d", i), minio.GetObjectOptions{})
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(object)
		_ = object.Close()
		if err != nil || string(body) != "s" {
			t.Fatalf("backup %d did not round trip: %q %v", i, body, err)
		}
	}
}
