package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var ytIDRe = regexp.MustCompile(`(?i)^[\w-]{11}$`)

func resolveYouTube(ctx context.Context, u *url.URL, opt Options) (Media, error) {
	id := youtubeVideoID(u)
	if id == "" {
		return Media{}, Err{Code: CodeLinkInvalid, Message: "youtube id missing"}
	}

	player, err := youtubePlayer(ctx, id)
	if err != nil {
		return Media{}, err
	}
	status, _ := player["playabilityStatus"].(map[string]any)
	if status != nil {
		st, _ := status["status"].(string)
		if st != "" && st != "OK" {
			reason, _ := status["reason"].(string)
			if reason == "" {
				reason = st
			}
			code := CodeUnavailable
			low := strings.ToLower(reason)
			switch {
			case strings.Contains(low, "private"):
				code = CodePrivate
			case strings.Contains(low, "login") || strings.Contains(low, "sign in"):
				code = CodeAuthRequired
			case strings.Contains(low, "age"):
				code = CodeAgeRestricted
			}
			return Media{}, Err{Code: code, Message: reason}
		}
	}

	streaming, _ := player["streamingData"].(map[string]any)
	if streaming == nil {
		return Media{}, Err{Code: CodeFetchEmpty, Message: "no youtube streams"}
	}

	title := "youtube_" + id
	if vd, ok := player["videoDetails"].(map[string]any); ok {
		if t, ok := vd["title"].(string); ok && t != "" {
			title = sanitizeName(t)
		}
	}

	if opt.Mode == "audio" {
		audio := pickAdaptive(streaming, "audio", opt.MaxHeight)
		if audio == "" {
			return Media{}, Err{Code: CodeFetchEmpty, Message: "no audio stream"}
		}
		return Media{
			URL:      audio,
			Filename: title + ".m4a",
			Headers:  youtubeDownloadHeaders(),
		}, nil
	}

	// Prefer progressive (muxed) ≤ height; else adaptive video+audio (caller downloads video only if no ffmpeg merge — we pick muxed or best single).
	if muxed := pickMuxed(streaming, opt.MaxHeight); muxed != "" {
		return Media{
			URL:      muxed,
			Filename: title + ".mp4",
			Headers:  youtubeDownloadHeaders(),
		}, nil
	}
	video := pickAdaptive(streaming, "video", opt.MaxHeight)
	audio := pickAdaptive(streaming, "audio", opt.MaxHeight)
	if video == "" {
		return Media{}, Err{Code: CodeFetchEmpty, Message: "no video stream"}
	}
	return Media{
		URL:      video,
		AudioURL: audio,
		Filename: title + ".mp4",
		Headers:  youtubeDownloadHeaders(),
	}, nil
}

func youtubeVideoID(u *url.URL) string {
	if v := u.Query().Get("v"); ytIDRe.MatchString(v) {
		return v
	}
	parts := pathParts(u)
	for i, p := range parts {
		if (p == "shorts" || p == "embed" || p == "v" || p == "live") && i+1 < len(parts) && ytIDRe.MatchString(parts[i+1]) {
			return parts[i+1]
		}
		if ytIDRe.MatchString(p) && (u.Hostname() == "youtu.be" || strings.Contains(u.Hostname(), "youtube")) {
			if u.Hostname() == "youtu.be" {
				return p
			}
		}
	}
	if u.Hostname() == "youtu.be" && len(parts) >= 1 && ytIDRe.MatchString(parts[0]) {
		return parts[0]
	}
	return ""
}

func youtubePlayer(ctx context.Context, id string) (map[string]any, error) {
	// ANDROID Innertube client — URLs come pre-deciphered (Cobalt/IOS/ANDROID approach).
	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":       "ANDROID",
				"clientVersion":    "20.10.38",
				"androidSdkVersion": 30,
				"hl":               "en",
				"gl":               "US",
				"userAgent":        "com.google.android.youtube/20.10.38 (Linux; U; Android 14) gzip",
				"platform":         "MOBILE",
			},
		},
		"videoId": id,
		"playbackContext": map[string]any{
			"contentPlaybackContext": map[string]any{
				"html5Preference": "HTML5_PREF_WANTS",
			},
		},
		"contentCheckOk": true,
		"racyCheckOk":    true,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.youtube.com/youtubei/v1/player?prettyPrint=false", bytes.NewReader(body))
	if err != nil {
		return nil, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "com.google.android.youtube/20.10.38 (Linux; U; Android 14) gzip")
	req.Header.Set("X-YouTube-Client-Name", "3")
	req.Header.Set("X-YouTube-Client-Version", "20.10.38")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, Err{Code: CodeFetchFail, Message: "bad youtube player response"}
	}
	return out, nil
}

func youtubeDownloadHeaders() map[string]string {
	return map[string]string{
		"User-Agent": ChromeUA,
		"Origin":     "https://www.youtube.com",
		"Referer":    "https://www.youtube.com/",
		"Accept":     "*/*",
	}
}

type ytFormat struct {
	URL       string
	Height    int
	Width     int
	Bitrate   int
	Mime      string
	HasVideo  bool
	HasAudio  bool
}

func parseFormats(streaming map[string]any) []ytFormat {
	var out []ytFormat
	for _, key := range []string{"formats", "adaptiveFormats"} {
		arr, _ := streaming[key].([]any)
		for _, it := range arr {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			u, _ := m["url"].(string)
			if u == "" {
				continue // ciphered — skip (ANDROID should not cipher)
			}
			mime, _ := m["mimeType"].(string)
			f := ytFormat{
				URL:     u,
				Mime:    mime,
				Height:  asInt(m["height"]),
				Width:   asInt(m["width"]),
				Bitrate: asInt(m["bitrate"]),
			}
			f.HasVideo = strings.HasPrefix(mime, "video/")
			f.HasAudio = strings.HasPrefix(mime, "audio/") || strings.Contains(mime, "mp4a")
			if key == "formats" {
				f.HasVideo = true
				f.HasAudio = true
			}
			out = append(out, f)
		}
	}
	return out
}

func pickMuxed(streaming map[string]any, maxH int) string {
	var best ytFormat
	for _, f := range parseFormats(streaming) {
		if !(f.HasVideo && f.HasAudio) {
			continue
		}
		h := f.Height
		if h == 0 {
			h = f.Width
		}
		if maxH > 0 && h > maxH {
			continue
		}
		if best.URL == "" || f.Bitrate > best.Bitrate {
			best = f
		}
	}
	return best.URL
}

func pickAdaptive(streaming map[string]any, kind string, maxH int) string {
	var best ytFormat
	for _, f := range parseFormats(streaming) {
		if kind == "video" {
			if !f.HasVideo || f.HasAudio && strings.HasPrefix(f.Mime, "audio/") {
				continue
			}
			if strings.HasPrefix(f.Mime, "audio/") {
				continue
			}
			h := f.Height
			if h == 0 {
				h = f.Width
			}
			if maxH > 0 && h > maxH {
				continue
			}
		} else {
			if !strings.HasPrefix(f.Mime, "audio/") {
				continue
			}
		}
		if best.URL == "" || f.Bitrate > best.Bitrate {
			best = f
		}
	}
	return best.URL
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		var n int
		fmt.Sscanf(t, "%d", &n)
		return n
	default:
		return 0
	}
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		s = s[:120]
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "youtube"
	}
	return out
}
