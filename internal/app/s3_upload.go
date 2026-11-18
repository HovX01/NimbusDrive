package app

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

// shortcut: limits are per Nimbus process; share a limiter if replicas use the same Telegram account.
type s3UploadLimiter struct {
	once  sync.Once
	slots chan struct{}
	mu    sync.Mutex
	next  time.Time
}

func (l *s3UploadLimiter) acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.once.Do(func() { l.slots = make(chan struct{}, 2) })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case l.slots <- struct{}{}:
		return func() { <-l.slots }, nil
	}
}

func (l *s3UploadLimiter) wait(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		delay := time.Until(l.next)
		if delay <= 0 {
			l.next = time.Now().Add(time.Second)
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *s3UploadLimiter) pause(delay time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until := time.Now().Add(delay + time.Second); until.After(l.next) {
		l.next = until
	}
}

func (s *Services) uploadS3Part(ctx context.Context, channelID int64, partNo int, name string, chunk []byte) (int, error) {
	for attempt := 0; ; attempt++ {
		if err := s.s3Uploads.wait(ctx); err != nil {
			return 0, err
		}
		id, err := s.Blobs.UploadPart(ctx, channelID, partNo, name, bytes.NewReader(chunk), int64(len(chunk)))
		var limited *domain.RateLimitError
		if !errors.As(err, &limited) {
			return id, err
		}
		s.s3Uploads.pause(limited.RetryAfter)
		if attempt == 4 {
			return 0, err
		}
	}
}
