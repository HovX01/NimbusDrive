package fetch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	fbHD = regexp.MustCompile(`"browser_native_hd_url":("(?:\\.|[^"\\])*")`)
	fbSD = regexp.MustCompile(`"browser_native_sd_url":("(?:\\.|[^"\\])*")`)
)

func resolveFacebook(ctx context.Context, u *url.URL, opt Options) (Media, error) {
	page := u.String()
	client := newHTTPClient()

	if strings.Contains(strings.ToLower(u.Hostname()), "fb.watch") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, page, nil)
		if err != nil {
			return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
		}
		req.Header = chromeHeaders()
		noRedir := *client
		noRedir.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		res, err := noRedir.Do(req)
		if err != nil {
			return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
		}
		loc := res.Header.Get("Location")
		res.Body.Close()
		if loc != "" {
			page = loc
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page, nil)
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	req.Header = chromeHeaders()
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	if opt.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+opt.AuthToken)
	}

	res, err := client.Do(req)
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	html := string(raw)

	mediaURL := firstJSONString(fbHD.FindStringSubmatch(html))
	if mediaURL == "" {
		mediaURL = firstJSONString(fbSD.FindStringSubmatch(html))
	}
	if mediaURL == "" {
		return Media{}, Err{Code: CodeFetchEmpty, Message: "no facebook video url"}
	}
	name := "facebook.mp4"
	if parts := pathParts(mustURL(page)); len(parts) > 0 {
		name = "facebook_" + parts[len(parts)-1] + ".mp4"
	}
	return Media{
		URL:      mediaURL,
		Filename: name,
		Headers:  bearerHeaders(opt.AuthToken),
	}, nil
}

func bearerHeaders(token string) map[string]string {
	headers := map[string]string{"User-Agent": ChromeUA}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return headers
}

func firstJSONString(m []string) string {
	if len(m) < 2 {
		return ""
	}
	var s string
	if err := json.Unmarshal([]byte(m[1]), &s); err != nil {
		return ""
	}
	return s
}
