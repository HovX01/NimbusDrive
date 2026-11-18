package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

// shortcut: limits are per Nimbus process; share a limiter if replicas use the same Telegram account.
type s3UploadLimiter struct {
	once    sync.Once
	slots   chan struct{}
	buffers sync.Pool
	mu      sync.Mutex
	next    time.Time
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

func (l *s3UploadLimiter) readChunk(r io.Reader, chunkSize int) ([]byte, func(), error) {
	const smallChunk = 256 << 10
	chunk, err := io.ReadAll(io.LimitReader(r, int64(min(chunkSize, smallChunk))))
	if err != nil || len(chunk) < smallChunk || len(chunk) == chunkSize {
		return chunk, nil, err
	}
	buf, _ := l.buffers.Get().(*[]byte)
	if buf == nil || cap(*buf) != chunkSize {
		b := make([]byte, chunkSize)
		buf = &b
	}
	n := copy(*buf, chunk)
	for n < chunkSize {
		var read int
		read, err = r.Read((*buf)[n:])
		n += read
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			break
		}
	}
	return (*buf)[:n], func() { l.buffers.Put(buf) }, err
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
