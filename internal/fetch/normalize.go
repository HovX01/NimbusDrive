package fetch

import (
	"fmt"
	"net/url"
	"strings"
)

// keepQueryKeys are query params worth keeping after clean (Cobalt-style).
var keepQueryKeys = map[string]struct{}{
	"v": {}, "z": {}, "p": {}, "list": {}, "index": {},
}

// NormalizeURL expands short links and strips tracking junk before download.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("url required")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "", fmt.Errorf("url must start with http:// or https://")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid url")
	}
	u = aliasURL(u)
	u = cleanURL(u)
	return u.String(), nil
}

func aliasURL(u *url.URL) *url.URL {
	host := strings.ToLower(u.Hostname())
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")

	switch {
	case host == "youtu.be" && len(parts) >= 1 && parts[0] != "":
		return mustParse("https://www.youtube.com/watch?v=" + url.QueryEscape(parts[0]))

	case (host == "youtube.com" || host == "www.youtube.com" || host == "m.youtube.com") &&
		len(parts) >= 2 && (parts[0] == "shorts" || parts[0] == "live"):
		return mustParse("https://www.youtube.com/watch?v=" + url.QueryEscape(parts[1]))

	case host == "x.com" || host == "www.x.com" || host == "mobile.twitter.com" ||
		host == "vxtwitter.com" || host == "fixvx.com":
		u2 := *u
		u2.Host = "twitter.com"
		return &u2

	case host == "vm.tiktok.com" || host == "vt.tiktok.com" || host == "m.tiktok.com":
		u2 := *u
		u2.Scheme = "https"
		return &u2

	case host == "dai.ly" && len(parts) >= 1:
		return mustParse("https://www.dailymotion.com/video/" + url.PathEscape(parts[0]))

	case host == "v.redd.it" && len(parts) >= 1:
		return mustParse("https://www.reddit.com/video/" + url.PathEscape(parts[0]))

	case host == "clips.twitch.tv" && len(parts) >= 1:
		return mustParse("https://www.twitch.tv/_/clip/" + url.PathEscape(parts[0]))
	}
	return u
}

func cleanURL(u *url.URL) *url.URL {
	q := u.Query()
	if len(q) == 0 {
		u.Fragment = ""
		return u
	}
	kept := url.Values{}
	for k, vals := range q {
		lk := strings.ToLower(k)
		if _, ok := keepQueryKeys[lk]; ok {
			kept[k] = vals
		}
	}
	u2 := *u
	u2.RawQuery = kept.Encode()
	u2.Fragment = ""
	return &u2
}

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

// DetectService returns a short service name for UI / error context.
func DetectService(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "unknown"
	}
	h := strings.ToLower(u.Hostname())
	switch {
	case strings.Contains(h, "youtube") || h == "youtu.be":
		return "youtube"
	case strings.Contains(h, "tiktok"):
		return "tiktok"
	case strings.Contains(h, "instagram"):
		return "instagram"
	case strings.Contains(h, "twitter") || h == "x.com" || strings.HasSuffix(h, ".x.com"):
		return "twitter"
	case strings.Contains(h, "reddit"):
		return "reddit"
	case strings.Contains(h, "vimeo"):
		return "vimeo"
	case strings.Contains(h, "soundcloud"):
		return "soundcloud"
	case strings.Contains(h, "facebook") || h == "fb.watch" || strings.HasSuffix(h, ".facebook.com"):
		return "facebook"
	case strings.Contains(h, "twitch"):
		return "twitch"
	default:
		return "web"
	}
}
