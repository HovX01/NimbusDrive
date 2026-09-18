package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func resolveTwitter(ctx context.Context, u *url.URL, opt Options) (Media, error) {
	id := twitterStatusID(u)
	if id == "" {
		return Media{}, Err{Code: CodeLinkInvalid, Message: "tweet id missing"}
	}

	// Syndication fallback (no guest GraphQL churn) — Cobalt secondary path.
	token := twitterSyndicationToken(id)
	api := fmt.Sprintf("https://cdn.syndication.twimg.com/tweet-result?id=%s&token=%s", id, url.QueryEscape(token))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	req.Header.Set("User-Agent", ChromeUA)
	req.Header.Set("Accept", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return Media{}, Err{Code: CodeUnavailable, Message: "tweet unavailable"}
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: "bad twitter response"}
	}

	mediaURL, filename, items := twitterAllMedia(data, id, opt)
	if mediaURL == "" && len(items) == 0 {
		return Media{}, Err{Code: CodeFetchEmpty, Message: "no media on this tweet"}
	}
	return Media{
		URL:      mediaURL,
		Filename: filename,
		Headers:  map[string]string{"User-Agent": ChromeUA},
		Items:    items,
	}, nil
}

func twitterStatusID(u *url.URL) string {
	parts := pathParts(u)
	for i, p := range parts {
		if (p == "status" || p == "statuses") && i+1 < len(parts) {
			id := parts[i+1]
			if _, err := strconv.ParseUint(id, 10, 64); err == nil {
				return id
			}
		}
	}
	if id := u.Query().Get("post_id"); id != "" {
		return id
	}
	return ""
}

// Cobalt/fxTwitter style: token = ((id/1e15)*π).toString(36) with 0 and . stripped.
func twitterSyndicationToken(id string) string {
	n, err := strconv.ParseFloat(id, 64)
	if err != nil {
		return id
	}
	frac := (n / 1e15) * math.Pi
	return toBase36Float(frac)
}

func toBase36Float(f float64) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if f <= 0 {
		return "0"
	}
	// Convert using JS-like algorithm for positive floats
	intPart := math.Floor(f)
	frac := f - intPart
	var ib strings.Builder
	if intPart == 0 {
		ib.WriteByte('0')
	} else {
		n := int64(intPart)
		var digits []byte
		for n > 0 {
			digits = append(digits, alphabet[n%36])
			n /= 36
		}
		for i := len(digits) - 1; i >= 0; i-- {
			ib.WriteByte(digits[i])
		}
	}
	if frac == 0 {
		out := ib.String()
		out = strings.ReplaceAll(out, "0", "")
		out = strings.ReplaceAll(out, ".", "")
		return out
	}
	ib.WriteByte('.')
	for i := 0; i < 12 && frac > 0; i++ {
		frac *= 36
		d := int(frac)
		ib.WriteByte(alphabet[d])
		frac -= float64(d)
	}
	out := ib.String()
	out = strings.ReplaceAll(out, "0", "")
	out = strings.ReplaceAll(out, ".", "")
	return out
}

func twitterAllMedia(data map[string]any, id string, opt Options) (string, string, []MediaItem) {
	hdr := map[string]string{"User-Agent": ChromeUA}
	var items []MediaItem
	photoN, videoN := 0, 0

	scan := func(list []any) {
		for _, it := range list {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			typ, _ := m["type"].(string)
			if typ == "photo" && opt.Mode != "audio" {
				if u, ok := m["media_url_https"].(string); ok && u != "" {
					photoN++
					items = append(items, MediaItem{
						URL:      u + "?name=4096x4096",
						Filename: fmt.Sprintf("twitter_%s_%d.jpg", id, photoN),
						Headers:  hdr,
					})
				}
				continue
			}
			vi, _ := m["video_info"].(map[string]any)
			if vi == nil {
				continue
			}
			vars, _ := vi["variants"].([]any)
			var bestURL string
			var bestBR int
			for _, v := range vars {
				vm, _ := v.(map[string]any)
				ct, _ := vm["content_type"].(string)
				if ct != "video/mp4" {
					continue
				}
				u, _ := vm["url"].(string)
				br := asInt(vm["bitrate"])
				if u != "" && br >= bestBR {
					bestBR = br
					bestURL = stripQueryTag(u)
				}
			}
			if bestURL == "" {
				continue
			}
			videoN++
			items = append(items, MediaItem{
				URL:      bestURL,
				Filename: fmt.Sprintf("twitter_%s_v%d.mp4", id, videoN),
				Headers:  hdr,
			})
		}
	}

	if md, ok := data["mediaDetails"].([]any); ok {
		scan(md)
	}
	if len(items) == 0 {
		if legacy, ok := data["legacy"].(map[string]any); ok {
			if ee, ok := legacy["extended_entities"].(map[string]any); ok {
				if md, ok := ee["media"].([]any); ok {
					scan(md)
				}
			}
		}
	}

	if opt.Mode == "audio" {
		// Audio-only: prefer first video track if any (no separate audio CDN).
		for _, it := range items {
			if strings.HasSuffix(strings.ToLower(it.Filename), ".mp4") {
				return it.URL, it.Filename, []MediaItem{it}
			}
		}
		return "", "", nil
	}
	if len(items) == 0 {
		return "", "", nil
	}
	return items[0].URL, items[0].Filename, items
}

func twitterBestMedia(data map[string]any, id string, opt Options) (string, string) {
	u, f, _ := twitterAllMedia(data, id, opt)
	return u, f
}

func stripQueryTag(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	q := parsed.Query()
	q.Del("tag")
	parsed.RawQuery = q.Encode()
	return parsed.String()
}
