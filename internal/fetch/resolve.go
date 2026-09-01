package fetch

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Resolve turns a page URL into a direct media URL (Cobalt-style extractors, no yt-dlp).
func Resolve(ctx context.Context, raw string, opt Options) (Media, error) {
	normalized, err := NormalizeURL(raw)
	if err != nil {
		return Media{}, Err{Code: CodeLinkInvalid, Message: err.Error()}
	}
	if opt.Mode == "" {
		opt.Mode = "video"
	}
	opt.MaxHeight = SnapHeight(opt.MaxHeight)
	service := DetectService(normalized)

	u, err := url.Parse(normalized)
	if err != nil {
		return Media{}, Err{Code: CodeLinkInvalid, Message: "invalid url"}
	}

	var m Media
	switch service {
	case "tiktok":
		m, err = resolveTikTok(ctx, u, opt)
	case "youtube":
		m, err = resolveYouTube(ctx, u, opt)
	case "twitter":
		m, err = resolveTwitter(ctx, u, opt)
	case "reddit":
		m, err = resolveReddit(ctx, u, opt)
	case "facebook":
		m, err = resolveFacebook(ctx, u, opt)
	default:
		err = Err{
			Code:    CodeLinkUnsupported,
			Message: fmt.Sprintf("no native extractor for %s yet", service),
		}
	}
	if err != nil {
		return wrapHybridFallback(ctx, normalized, opt, err)
	}
	m.Service = service
	if m.Filename == "" {
		m.Filename = "download.mp4"
	}
	return m, nil
}

// Err is a structured fetch failure.
type Err struct {
	Code    ErrorCode
	Message string
}

func (e Err) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

func pathParts(u *url.URL) []string {
	var out []string
	for _, p := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func AsClassified(err error) Classified {
	if err == nil {
		return Classified{}
	}
	if e, ok := err.(Err); ok {
		retry := e.Code == CodeFetchFail || e.Code == CodeTikTokBlocked || e.Code == CodeFetchRate
		msg := e.Message
		if msg == "" {
			msg = string(e.Code)
		}
		return Classified{Code: e.Code, Message: msg, Retry: retry}
	}
	return ClassifyYtDlpError(err.Error(), "")
}
