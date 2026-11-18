package app

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

type s3RetryBlobs struct {
	domain.BlobStore
	upload func(context.Context, io.Reader) (int, error)
}

func (b s3RetryBlobs) UploadPart(ctx context.Context, _ int64, _ int, _ string, r io.Reader, _ int64) (int, error) {
	return b.upload(ctx, r)
}

func TestS3RetriesOnlyExplicitRateLimitsAndReplaysChunk(t *testing.T) {
	limited := &domain.RateLimitError{Err: errors.New("rate limited")}
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"rate limit", limited, 2},
		{"uncertain network failure", errors.New("connection reset"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			start := time.Now()
			svc := &Services{Blobs: s3RetryBlobs{upload: func(_ context.Context, r io.Reader) (int, error) {
				attempts++
				body, err := io.ReadAll(r)
				if err != nil || string(body) != "backup chunk" {
					t.Errorf("attempt %d: body=%q err=%v", attempts, body, err)
				}
				if attempts == 1 {
					return 0, tc.err
				}
				return 42, nil
			}}}
			id, err := svc.uploadS3Part(context.Background(), 1, 0, "backup", []byte("backup chunk"))
			if attempts != tc.want {
				t.Fatalf("attempts=%d, want %d", attempts, tc.want)
			}
			if tc.want == 2 {
				if id != 42 || err != nil || time.Since(start) < time.Second {
					t.Fatalf("rate-limit retry: id=%d err=%v elapsed=%v", id, err, time.Since(start))
				}
			} else if !errors.Is(err, tc.err) {
				t.Fatalf("lost original failure: %v", err)
			}
		})
	}
}

func TestS3CooldownCancelsWithoutRetrying(t *testing.T) {
	attempts := 0
	svc := &Services{Blobs: s3RetryBlobs{upload: func(context.Context, io.Reader) (int, error) {
		attempts++
		return 0, &domain.RateLimitError{Err: errors.New("FLOOD_WAIT"), RetryAfter: time.Hour}
	}}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := svc.uploadS3Part(ctx, 1, 0, "backup", []byte("data"))
	if !errors.Is(err, context.DeadlineExceeded) || attempts != 1 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
	other, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stop()
	if err := svc.s3Uploads.wait(other); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("other backup ignored shared cooldown: %v", err)
	}
}

func TestS3RateLimitRetriesAreBounded(t *testing.T) {
	attempts := 0
	limited := &domain.RateLimitError{Err: errors.New("rate limited")}
	svc := &Services{Blobs: s3RetryBlobs{upload: func(context.Context, io.Reader) (int, error) {
		attempts++
		return 0, limited
	}}}
	_, err := svc.uploadS3Part(context.Background(), 1, 0, "backup", []byte("data"))
	if attempts != 5 || !errors.Is(err, limited) {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

func TestS3QueuedUploadCancelsAndReleasesSlots(t *testing.T) {
	var limiter s3UploadLimiter
	for range 2 {
		release, err := limiter.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := limiter.acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third upload bypassed shared limit: %v", err)
	}
}
