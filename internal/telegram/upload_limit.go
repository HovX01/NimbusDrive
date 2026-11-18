package telegram

import (
	"github.com/gotd/td/tgerr"
	"github.com/vrc/nimbus/internal/domain"
)

func uploadRateLimit(err error) error {
	if delay, ok := tgerr.AsFloodWait(err); ok {
		return &domain.RateLimitError{Err: err, RetryAfter: delay}
	}
	return err
}
