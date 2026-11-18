package telegram

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gotd/td/tgerr"
	"github.com/vrc/nimbus/internal/domain"
)

func TestUploadRateLimitPreservesTelegramWait(t *testing.T) {
	original := fmt.Errorf("upload: %w", tgerr.New(420, "FLOOD_WAIT_17"))
	var limited *domain.RateLimitError
	if err := uploadRateLimit(original); !errors.As(err, &limited) || limited.RetryAfter != 17*time.Second || !errors.Is(err, original) {
		t.Fatalf("lost flood wait or original error: %v", err)
	}
	other := errors.New("connection reset")
	if uploadRateLimit(other) != other {
		t.Fatal("uncertain failure became retryable")
	}
}
