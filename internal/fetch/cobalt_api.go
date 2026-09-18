package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// resolveViaCobaltAPI calls a Cobalt-compatible instance (NIMBUS_COBALT_API).
//
// Set NIMBUS_COBALT_API to the API base URL (must accept POST / with JSON), e.g. a
// self-hosted https://github.com/imputnet/cobalt deploy. Optional NIMBUS_COBALT_API_KEY
// sends Authorization: Api-Key …. Frontend-only hosts (e.g. cobalt.canine.tools) reject POST.
func resolveViaCobaltAPI(ctx context.Context, pageURL string, opt Options) (Media, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("NIMBUS_COBALT_API")), "/")
	if base == "" {
		return Media{}, Err{Code: CodeLinkUnsupported, Message: "cobalt api not configured"}
	}

	h := SnapHeight(opt.MaxHeight)
	quality := strconv.Itoa(h)
	if h >= 2160 {
		quality = "max" // highest available from the source
	}
	mode := "auto"
	if opt.Mode == "audio" {
		mode = "audio"
	}
	body, _ := json.Marshal(map[string]any{
		"url":           pageURL,
		"videoQuality":  quality,
		"downloadMode":  mode,
		"filenameStyle": "basic",
		// Tunnel through Cobalt so Instagram/CDN hotlink blocks don't kill downloads.
		"alwaysProxy": true,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/", bytes.NewReader(body))
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", CobaltUA)
	if key := strings.TrimSpace(os.Getenv("NIMBUS_COBALT_API_KEY")); key != "" {
		req.Header.Set("Authorization", "Api-Key "+key)
	}

	client := &http.Client{Timeout: 45 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}

	var out struct {
		Status        string   `json:"status"`
		URL           string   `json:"url"`
		Filename      string   `json:"filename"`
		Audio         string   `json:"audio"`
		AudioFilename string   `json:"audioFilename"`
		Tunnel        []string `json:"tunnel"`
		Output        struct {
			Filename string `json:"filename"`
			Type     string `json:"type"`
		} `json:"output"`
		Picker []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"picker"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: "bad cobalt response"}
	}

	switch out.Status {
	case "tunnel", "redirect":
		if out.URL == "" {
			return Media{}, Err{Code: CodeFetchFail, Message: "cobalt returned empty url"}
		}
		name := out.Filename
		if name == "" {
			name = "cobalt_download.bin"
		}
		name = ensureMediaFilename(name, out.URL, "")
		return Media{
			URL:      rewriteCobaltURL(base, out.URL),
			Filename: name,
			Headers:  map[string]string{"User-Agent": ChromeUA},
			Service:  "cobalt",
		}, nil

	case "local-processing":
		// Prefer a single tunnel stream when Cobalt asks the client to remux.
		u := ""
		if len(out.Tunnel) > 0 {
			u = out.Tunnel[0]
		}
		if u == "" {
			return Media{}, Err{Code: CodeFetchFail, Message: "cobalt local-processing has no tunnel"}
		}
		name := out.Output.Filename
		if name == "" {
			name = out.Filename
		}
		if name == "" {
			name = "cobalt_download.bin"
		}
		name = ensureMediaFilename(name, u, out.Output.Type)
		return Media{
			URL:      rewriteCobaltURL(base, u),
			Filename: name,
			Headers:  map[string]string{"User-Agent": ChromeUA},
			Service:  "cobalt",
		}, nil

	case "picker":
		if opt.Mode == "audio" {
			if out.Audio == "" {
				return Media{}, Err{Code: CodeFetchEmpty, Message: "cobalt picker has no audio"}
			}
			name := out.AudioFilename
			if name == "" {
				name = "cobalt_audio.mp3"
			}
			return Media{
				URL:      rewriteCobaltURL(base, out.Audio),
				Filename: name,
				Headers:  map[string]string{"User-Agent": ChromeUA},
				Service:  "cobalt",
			}, nil
		}
		hdr := map[string]string{"User-Agent": ChromeUA}
		items := make([]MediaItem, 0, len(out.Picker))
		photoN, videoN, liveN := 0, 0, 0
		lastWasPhoto := false
		for _, item := range out.Picker {
			if item.URL == "" {
				continue
			}
			typ := strings.ToLower(strings.TrimSpace(item.Type))
			if typ == "" {
				typ = guessPickerType(item.URL)
			}
			var name string
			switch typ {
			case "photo", "image":
				photoN++
				name = fmt.Sprintf("cobalt_photo_%d.jpg", photoN)
				lastWasPhoto = true
			case "gif":
				// Cobalt uses "gif" for animated / Live Photo motion (mp4).
				liveN++
				name = fmt.Sprintf("cobalt_live_%d.mp4", liveN)
				lastWasPhoto = false
			default:
				videoN++
				// Photo followed by video ≈ Instagram / iOS Live Photo pair.
				if lastWasPhoto {
					liveN++
					name = fmt.Sprintf("cobalt_live_%d.mp4", liveN)
				} else {
					name = fmt.Sprintf("cobalt_video_%d.mp4", videoN)
				}
				lastWasPhoto = false
			}
			items = append(items, MediaItem{
				URL:      rewriteCobaltURL(base, item.URL),
				Filename: name,
				Headers:  hdr,
			})
		}
		if len(items) == 0 {
			return Media{}, Err{Code: CodeFetchEmpty, Message: "cobalt picker empty"}
		}
		m := Media{
			URL:      items[0].URL,
			Filename: items[0].Filename,
			Headers:  hdr,
			Service:  "cobalt",
			Items:    items,
		}
		return m, nil

	case "error", "":
		code := CodeFetchFail
		msg := "cobalt instance failed"
		if out.Error.Code != "" {
			msg = out.Error.Code
			switch {
			case strings.Contains(out.Error.Code, "unsupported"):
				code = CodeLinkUnsupported
			case strings.Contains(out.Error.Code, "private"):
				code = CodePrivate
			case strings.Contains(out.Error.Code, "age"):
				code = CodeAgeRestricted
			case strings.Contains(out.Error.Code, "auth"):
				code = CodeFetchFail
				msg = "cobalt requires auth (set NIMBUS_COBALT_API_KEY or use an open instance)"
			case strings.Contains(out.Error.Code, "fetch.empty"),
				strings.Contains(out.Error.Code, "fetch.fail"):
				code = CodeFetchEmpty
				msg = "could not extract media (private post, or Instagram needs cookies)"
			}
		}
		return Media{}, Err{Code: code, Message: msg}

	default:
		return Media{}, Err{Code: CodeFetchFail, Message: "unsupported cobalt status: " + out.Status}
	}
}

func cobaltAPIConfigured() bool {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("NIMBUS_COBALT_API")), "/") != ""
}

func wrapHybridFallback(ctx context.Context, pageURL string, opt Options, nativeErr error) (Media, error) {
	var fallbackErr error

	if cobaltAPIConfigured() {
		m, err := resolveViaCobaltAPI(ctx, pageURL, opt)
		if err == nil {
			return m, nil
		}
		fallbackErr = joinFallbackErr(fallbackErr, "Cobalt", err)
	}

	if vidbeeAPIConfigured() {
		m, err := resolveViaVidBee(ctx, pageURL, opt)
		if err == nil {
			return m, nil
		}
		fallbackErr = joinFallbackErr(fallbackErr, "VidBee", err)
	}

	if ytDLPConfigured() {
		m, err := resolveViaYtDLP(ctx, pageURL, opt)
		if err == nil {
			return m, nil
		}
		fallbackErr = joinFallbackErr(fallbackErr, "yt-dlp", err)
	}

	if fallbackErr != nil {
		return Media{}, Err{
			Code:    CodeFetchFail,
			Message: fmt.Sprintf("%v — %v", nativeErr, fallbackErr),
		}
	}
	return Media{}, nativeErr
}

func joinFallbackErr(prev error, label string, err error) error {
	msg := err.Error()
	if e, ok := err.(Err); ok && e.Message != "" {
		msg = e.Message
	}
	part := fmt.Sprintf("%s fallback failed: %s", label, msg)
	if prev == nil {
		return fmt.Errorf("%s", part)
	}
	return fmt.Errorf("%v — %s", prev, part)
}

// rewriteCobaltURL keeps tunnel URLs on the Docker-network Cobalt host.
func rewriteCobaltURL(apiBase, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || apiBase == "" {
		return raw
	}
	if strings.HasPrefix(raw, "/") {
		return apiBase + raw
	}
	return raw
}

func guessPickerType(u string) string {
	ext := strings.ToLower(path.Ext(strings.Split(u, "?")[0]))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".bmp":
		return "photo"
	case ".gif":
		return "gif"
	case ".mp4", ".webm", ".mov", ".m4v":
		return "video"
	default:
		// Instagram/X carousels often omit type; prefer photo so files preview correctly.
		return "photo"
	}
}

func ensureMediaFilename(name, mediaURL, mime string) string {
	name = path.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		name = "cobalt_download.bin"
	}
	ext := strings.ToLower(path.Ext(name))
	if ext != "" && ext != ".bin" && ext != ".tmp" {
		return name
	}
	if e := extFromContentType(mime); e != "" {
		return strings.TrimSuffix(name, ext) + e
	}
	if e := strings.ToLower(path.Ext(strings.Split(mediaURL, "?")[0])); e != "" && len(e) <= 5 {
		return strings.TrimSuffix(name, ext) + e
	}
	return name
}

func extFromContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch ct {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/heic", "image/heif":
		return ".heic"
	case "video/mp4", "application/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	default:
		return ""
	}
}
