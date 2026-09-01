package fetch

import (
	"strings"
)

// ErrorCode is a stable machine-readable fetch failure (Cobalt-style).
type ErrorCode string

const (
	CodeOK              ErrorCode = ""
	CodeLinkInvalid     ErrorCode = "link.invalid"
	CodeLinkUnsupported ErrorCode = "link.unsupported"
	CodeFetchFail       ErrorCode = "fetch.fail"
	CodeFetchRate       ErrorCode = "fetch.rate"
	CodeFetchEmpty      ErrorCode = "fetch.empty"
	CodeAuthRequired    ErrorCode = "content.login_required"
	CodePrivate         ErrorCode = "content.private"
	CodeAgeRestricted   ErrorCode = "content.age"
	CodeGeoBlocked      ErrorCode = "content.region"
	CodeUnavailable     ErrorCode = "content.unavailable"
	CodeTooLong         ErrorCode = "content.too_long"
	CodeTikTokBlocked   ErrorCode = "fetch.tiktok_blocked"
	CodeEngineMissing   ErrorCode = "engine.missing"
)

// Classified is a failure → code + user message.
type Classified struct {
	Code    ErrorCode
	Message string
	Retry   bool
}

// Classify maps a raw error string (legacy) to a stable code.
func Classify(raw string, service string) Classified {
	return ClassifyYtDlpError(raw, service)
}

// ClassifyYtDlpError kept name for older call sites; maps extractor text → code.
func ClassifyYtDlpError(raw string, service string) Classified {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return Classified{Code: CodeFetchFail, Message: "Download failed", Retry: true}
	}

	switch {
	case strings.Contains(text, "unsupported") || strings.Contains(text, "no native extractor"):
		return Classified{Code: CodeLinkUnsupported, Message: "This link type is not supported", Retry: false}

	case strings.Contains(text, "rate limit") || strings.Contains(text, "too many requests") || strings.Contains(text, "429"):
		return Classified{Code: CodeFetchRate, Message: "Rate limited — wait a bit and retry", Retry: true}

	case strings.Contains(text, "login") || strings.Contains(text, "sign in"):
		return Classified{Code: CodeAuthRequired, Message: "Login required", Retry: false}

	case strings.Contains(text, "private"):
		return Classified{Code: CodePrivate, Message: "This media is private", Retry: false}

	case strings.Contains(text, "age-restricted") || strings.Contains(text, "age restricted") ||
		strings.Contains(text, "confirm your age") || strings.Contains(text, "age gate"):
		return Classified{Code: CodeAgeRestricted, Message: "Age-restricted content", Retry: false}

	case strings.Contains(text, "universal data for rehydration") || strings.Contains(text, "please wait") ||
		strings.Contains(text, "tiktok waf") || strings.Contains(text, "tiktok blocked") ||
		(service == "tiktok" && strings.Contains(text, "no page data")):
		return Classified{
			Code:    CodeTikTokBlocked,
			Message: "TikTok blocked extraction — retry in a bit",
			Retry:   true,
		}

	case strings.Contains(text, "unavailable") || strings.Contains(text, "not found") || strings.Contains(text, "404"):
		return Classified{Code: CodeUnavailable, Message: "Media is unavailable", Retry: false}

	case strings.Contains(text, "timed out") || strings.Contains(text, "timeout") ||
		strings.Contains(text, "connection reset") || strings.Contains(text, "cdn http 403") ||
		strings.Contains(text, "cdn http 502") || strings.Contains(text, "cdn http 503"):
		return Classified{Code: CodeFetchFail, Message: "Temporary network error — retrying…", Retry: true}
	}

	line := lastLine(raw)
	if len(line) > 160 {
		line = line[:160] + "…"
	}
	return Classified{Code: CodeFetchFail, Message: line, Retry: true}
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return "Download failed"
}
