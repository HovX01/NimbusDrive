package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

	quality := strconv.Itoa(SnapHeight(opt.MaxHeight))
	mode := "auto"
	if opt.Mode == "audio" {
		mode = "audio"
	}
	body, _ := json.Marshal(map[string]any{
		"url":           pageURL,
		"videoQuality":  quality,
		"downloadMode":  mode,
		"filenameStyle": "basic",
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
		Status        string `json:"status"`
		URL           string `json:"url"`
		Filename      string `json:"filename"`
		Audio         string `json:"audio"`
		AudioFilename string `json:"audioFilename"`
		Picker        []struct {
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
			name = "cobalt_download.mp4"
		}
		return Media{
			URL:      out.URL,
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
				URL:      out.Audio,
				Filename: name,
				Headers:  map[string]string{"User-Agent": ChromeUA},
				Service:  "cobalt",
			}, nil
		}
		for _, item := range out.Picker {
			if item.URL == "" {
				continue
			}
			name := out.Filename
			if name == "" {
				switch item.Type {
				case "photo":
					name = "cobalt_photo.jpg"
				default:
					name = "cobalt_download.mp4"
				}
			}
			return Media{
				URL:      item.URL,
				Filename: name,
				Headers:  map[string]string{"User-Agent": ChromeUA},
				Service:  "cobalt",
			}, nil
		}
		return Media{}, Err{Code: CodeFetchEmpty, Message: "cobalt picker empty"}

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
