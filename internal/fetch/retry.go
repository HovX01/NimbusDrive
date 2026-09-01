package fetch

import "math"

const (
	DefaultMaxAttempts = 3
	TikTokMaxAttempts  = 6
	BaseBackoffMs      = 400
)

// MaxAttemptsFor returns how many download tries to allow for a service.
func MaxAttemptsFor(service string) int {
	if service == "tiktok" {
		return TikTokMaxAttempts
	}
	return DefaultMaxAttempts
}

// BackoffMs returns exponential backoff with deterministic jitter seed (attempt 0-based).
// delay = min(8000, base * 2^attempt) * (0.75 + 0.25*frac) where frac comes from seed.
func BackoffMs(attempt int, seed uint32) int {
	if attempt < 0 {
		attempt = 0
	}
	exp := BaseBackoffMs * int(math.Pow(2, float64(attempt)))
	if exp > 8000 {
		exp = 8000
	}
	// Simple LCG-ish fraction from seed+attempt for jitter in [0.75, 1.0]
	x := seed + uint32(attempt)*1103515245 + 12345
	frac := float64(x%1000) / 1000.0
	return int(float64(exp) * (0.75 + 0.25*frac))
}

// ShouldRetryAttempt returns true when another try is allowed.
func ShouldRetryAttempt(attempt, maxAttempts int, classified Classified) bool {
	if attempt+1 >= maxAttempts {
		return false
	}
	return classified.Retry
}
