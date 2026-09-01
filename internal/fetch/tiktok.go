package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var tikTokVideoID = regexp.MustCompile(`(?:video|photo)/(\d{5,25})`)

func resolveTikTok(ctx context.Context, u *url.URL, opt Options) (Media, error) {
	postID := extractTikTokID(u)
	isPhoto := strings.Contains(strings.ToLower(u.Path), "/photo/")
	client := newHTTPClient()

	if postID == "" {
		short := extractTikTokShort(u)
		if short == "" {
			return Media{}, Err{Code: CodeLinkInvalid, Message: "tiktok id missing"}
		}
		var err error
		var kind string
		postID, kind, err = resolveTikTokShort(ctx, client, short)
		if err != nil {
			return Media{}, err
		}
		isPhoto = kind == "photo"
	}

	// Cobalt always loads /video/ for HTML even for photos.
	pageURL := "https://www.tiktok.com/@i/video/" + postID
	detail, jarCookies, err := fetchTikTokDetail(ctx, client, pageURL, opt)
	if err != nil {
		return Media{}, err
	}

	author, _ := detail["author"].(map[string]any)
	unique := "tiktok"
	if author != nil {
		if v, ok := author["uniqueId"].(string); ok && v != "" {
			unique = v
		}
	}

	headers := map[string]string{
		"User-Agent": ChromeUA,
		"Referer":    "https://www.tiktok.com/",
		"Cookie":     jarCookies,
	}

	// Photo slideshow (e.g. vt.tiktok.com → /@user/photo/…)
	if imgURL := firstTikTokImage(detail); imgURL != "" && (isPhoto || playAddrOf(detail) == "") {
		if opt.Mode == "audio" {
			if music := musicURL(detail); music != "" {
				return Media{
					URL:      music,
					Filename: fmt.Sprintf("tiktok_%s_%s_audio.mp3", unique, postID),
					Headers:  headers,
				}, nil
			}
			return Media{}, Err{Code: CodeFetchEmpty, Message: "this TikTok photo has no audio track"}
		}
		return Media{
			URL:      imgURL,
			Filename: fmt.Sprintf("tiktok_%s_%s.jpg", unique, postID),
			Headers:  headers,
		}, nil
	}

	playAddr := playAddrOf(detail)
	filename := fmt.Sprintf("tiktok_%s_%s.mp4", unique, postID)
	if opt.Mode == "audio" {
		if music := musicURL(detail); music != "" {
			playAddr = music
		}
		filename = strings.TrimSuffix(filename, ".mp4") + "_audio.mp4"
	}
	if playAddr == "" {
		return Media{}, Err{Code: CodeFetchEmpty, Message: "no playable media on this TikTok"}
	}

	return Media{
		URL:      playAddr,
		Filename: filename,
		Headers:  headers,
	}, nil
}

func fetchTikTokDetail(ctx context.Context, client *http.Client, pageURL string, opt Options) (map[string]any, string, error) {
	uas := []string{ChromeUA, ShortLinkUA, CobaltUA}
	var lastErr error
	for _, ua := range uas {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return nil, "", Err{Code: CodeFetchFail, Message: err.Error()}
		}
		req.Header = chromeHeaders()
		req.Header.Set("User-Agent", ua)
		if ua == CobaltUA {
			req.Header.Set("Accept", "*/*")
		}
		_ = opt // cookies path reserved for future Netscape jar load

		res, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		res.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		detail, err := parseTikTokRehydration(string(body))
		if err == nil {
			return detail, cookieHeaderFor(client, mustURL("https://www.tiktok.com/")), nil
		}
		if e, ok := err.(Err); ok {
			return nil, "", e
		}
		lastErr = err
	}
	if lastErr != nil {
		if _, ok := lastErr.(Err); ok {
			return nil, "", lastErr
		}
	}
	return nil, "", Err{
		Code: CodeTikTokBlocked,
		Message: "TikTok blocked extraction from this network (WAF). " +
			"Open the link in a browser, or set NIMBUS_COBALT_API to a working Cobalt instance.",
	}
}

func playAddrOf(detail map[string]any) string {
	video, _ := detail["video"].(map[string]any)
	if video == nil {
		return ""
	}
	playAddr, _ := video["playAddr"].(string)
	return playAddr
}

func musicURL(detail map[string]any) string {
	music, _ := detail["music"].(map[string]any)
	if music == nil {
		return ""
	}
	pu, _ := music["playUrl"].(string)
	return pu
}

func firstTikTokImage(detail map[string]any) string {
	imagePost, _ := detail["imagePost"].(map[string]any)
	if imagePost == nil {
		return ""
	}
	images, _ := imagePost["images"].([]any)
	for _, it := range images {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		imageURL, _ := m["imageURL"].(map[string]any)
		if imageURL == nil {
			continue
		}
		list, _ := imageURL["urlList"].([]any)
		var fallback string
		for _, u := range list {
			s, _ := u.(string)
			if s == "" {
				continue
			}
			if strings.Contains(s, ".jpeg") || strings.Contains(s, ".jpg") || strings.Contains(s, ".webp") {
				return s
			}
			if fallback == "" {
				fallback = s
			}
		}
		if fallback != "" {
			return fallback
		}
	}
	return ""
}

func extractTikTokID(u *url.URL) string {
	if m := tikTokVideoID.FindStringSubmatch(u.Path); len(m) == 2 {
		return m[1]
	}
	return ""
}

func extractTikTokShort(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	parts := pathParts(u)
	if host == "vm.tiktok.com" || host == "vt.tiktok.com" || (host == "www.tiktok.com" && len(parts) >= 2 && parts[0] == "t") {
		if host == "www.tiktok.com" {
			return parts[1]
		}
		if len(parts) >= 1 {
			return parts[0]
		}
	}
	if (host == "www.tiktok.com" || host == "tiktok.com") && len(parts) == 1 && !strings.HasPrefix(parts[0], "@") {
		return parts[0]
	}
	return ""
}

func resolveTikTokShort(ctx context.Context, client *http.Client, short string) (id, kind string, err error) {
	shortURL := "https://vt.tiktok.com/" + short
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, shortURL, nil)
	if err != nil {
		return "", "", Err{Code: CodeFetchFail, Message: err.Error()}
	}
	req.Header.Set("User-Agent", ShortLinkUA)

	noRedir := *client
	noRedir.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	res, err := noRedir.Do(req)
	if err != nil {
		return "", "", Err{Code: CodeFetchFail, Message: err.Error()}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	html := string(body)

	try := func(loc string) (string, string, bool) {
		if loc == "" {
			return "", "", false
		}
		u := mustURL(loc)
		id := extractTikTokID(u)
		if id == "" {
			if nu, nerr := NormalizeURL(loc); nerr == nil {
				u = mustURL(nu)
				id = extractTikTokID(u)
			}
		}
		if id == "" {
			return "", "", false
		}
		kind := "video"
		if strings.Contains(strings.ToLower(u.Path), "/photo/") {
			kind = "photo"
		}
		return id, kind, true
	}

	if loc := res.Header.Get("Location"); loc != "" {
		if id, kind, ok := try(loc); ok {
			return id, kind, nil
		}
	}
	if strings.HasPrefix(html, `<a href="https://`) {
		rest := strings.TrimPrefix(html, `<a href="`)
		href := strings.Split(rest, `"`)[0]
		href = strings.Split(href, "?")[0]
		if id, kind, ok := try(href); ok {
			return id, kind, nil
		}
	}
	return "", "", Err{Code: CodeFetchFail, Message: "could not resolve TikTok short link"}
}

func parseTikTokRehydration(html string) (map[string]any, error) {
	const marker = `<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">`
	i := strings.Index(html, marker)
	if i < 0 {
		alt := `id="__UNIVERSAL_DATA_FOR_REHYDRATION__"`
		i = strings.Index(html, alt)
		if i < 0 {
			return nil, fmt.Errorf("no rehydration")
		}
		gt := strings.Index(html[i:], ">")
		if gt < 0 {
			return nil, fmt.Errorf("no rehydration")
		}
		i = i + gt + 1
	} else {
		i += len(marker)
	}
	j := strings.Index(html[i:], "</script>")
	if j < 0 {
		return nil, fmt.Errorf("no rehydration end")
	}
	raw := html[i : i+j]
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, err
	}
	scope, _ := data["__DEFAULT_SCOPE__"].(map[string]any)
	if scope == nil {
		return nil, fmt.Errorf("no scope")
	}
	vd, _ := scope["webapp.video-detail"].(map[string]any)
	if vd == nil {
		return nil, fmt.Errorf("no video-detail")
	}
	if msg, ok := vd["statusMsg"].(string); ok && msg != "" {
		return nil, Err{Code: CodeUnavailable, Message: "post unavailable"}
	}
	info, _ := vd["itemInfo"].(map[string]any)
	if info == nil {
		return nil, fmt.Errorf("no itemInfo")
	}
	detail, _ := info["itemStruct"].(map[string]any)
	if detail == nil {
		return nil, fmt.Errorf("no itemStruct")
	}
	if classified, _ := detail["isContentClassified"].(bool); classified {
		return nil, Err{Code: CodeAgeRestricted, Message: "age-restricted TikTok"}
	}
	if detail["author"] == nil {
		return nil, Err{Code: CodeFetchEmpty, Message: "empty post"}
	}
	return detail, nil
}

func cookieHeaderFor(c *http.Client, u *url.URL) string {
	if c == nil || c.Jar == nil || u == nil {
		return ""
	}
	var parts []string
	for _, ck := range c.Jar.Cookies(u) {
		parts = append(parts, ck.Name+"="+ck.Value)
	}
	return strings.Join(parts, "; ")
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		return &url.URL{}
	}
	return u
}
