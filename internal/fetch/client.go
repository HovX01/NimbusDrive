package fetch

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// Desktop Chrome UA — Cobalt genericUserAgent style.
const ChromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36"

// ShortLinkUA is Chrome UA without the "Chrome/…" token (Cobalt TikTok short-link bounce).
const ShortLinkUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko)"

// CobaltUA is used when browser-like UAs get WAF'd on some hosts.
const CobaltUA = "cobalt/nimbus (+https://github.com/imputnet/cobalt)"

// MediaItem is one file from a post (photo, video, gif).
type MediaItem struct {
	URL       string
	LocalPath string
	Filename  string
	Headers   map[string]string
}

// Media is a resolved CDN URL ready to download (Cobalt extractor result).
// LocalPath is set when a sidecar (VidBee) already saved the file on disk.
// Items holds every media file from multi-image/video posts when available.
type Media struct {
	URL       string
	LocalPath string
	AudioURL  string // optional second stream (YouTube adaptive / Reddit)
	Filename  string
	Headers   map[string]string
	Service   string
	Items     []MediaItem
}

// AllItems returns gallery items, or a single item from URL/LocalPath.
func (m Media) AllItems() []MediaItem {
	if len(m.Items) > 0 {
		return m.Items
	}
	if m.URL != "" || m.LocalPath != "" {
		return []MediaItem{{URL: m.URL, LocalPath: m.LocalPath, Filename: m.Filename, Headers: m.Headers}}
	}
	return nil
}

// Options control quality / mode.
type Options struct {
	Mode       string // video|audio  (video = video+photos from posts)
	MaxHeight  int
	Cookies    string // optional Netscape cookies path
	AuthToken  string // optional OAuth bearer for connected provider APIs
	TempDir    string // optional resolver work dir for local download engines
	OnProgress ProgressFunc
}

func newHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 90 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

func chromeHeaders() http.Header {
	h := make(http.Header)
	h.Set("User-Agent", ChromeUA)
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	h.Set("Accept-Language", "en-US,en;q=0.9")
	return h
}
