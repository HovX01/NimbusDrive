package fetch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func resolveReddit(ctx context.Context, u *url.URL, opt Options) (Media, error) {
	id := redditPostID(u)
	client := newHTTPClient()
	if id == "" {
		// follow redirects for v.redd.it / share links
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
		}
		req.Header.Set("User-Agent", ChromeUA)
		res, err := client.Do(req)
		if err != nil {
			return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
		}
		res.Body.Close()
		id = redditPostID(res.Request.URL)
	}
	if id == "" {
		return Media{}, Err{Code: CodeLinkInvalid, Message: "reddit id missing"}
	}

	api := "https://www.reddit.com/comments/" + id + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	req.Header.Set("User-Agent", ChromeUA)
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	var tree []any
	if err := json.Unmarshal(raw, &tree); err != nil || len(tree) == 0 {
		return Media{}, Err{Code: CodeFetchFail, Message: "bad reddit response"}
	}
	listing, _ := tree[0].(map[string]any)
	data, _ := listing["data"].(map[string]any)
	children, _ := data["children"].([]any)
	if len(children) == 0 {
		return Media{}, Err{Code: CodeFetchEmpty, Message: "empty reddit post"}
	}
	child, _ := children[0].(map[string]any)
	post, _ := child["data"].(map[string]any)
	if post == nil {
		return Media{}, Err{Code: CodeFetchEmpty, Message: "empty reddit post"}
	}

	if urlStr, ok := post["url"].(string); ok && strings.HasSuffix(strings.ToLower(urlStr), ".gif") {
		return Media{URL: urlStr, Filename: "reddit_" + id + ".gif", Headers: map[string]string{"User-Agent": ChromeUA}}, nil
	}

	sm, _ := post["secure_media"].(map[string]any)
	if sm == nil {
		sm, _ = post["media"].(map[string]any)
	}
	rv, _ := sm["reddit_video"].(map[string]any)
	if rv == nil {
		return Media{}, Err{Code: CodeFetchEmpty, Message: "no reddit video"}
	}
	fallback, _ := rv["fallback_url"].(string)
	if fallback == "" {
		return Media{}, Err{Code: CodeFetchEmpty, Message: "no reddit video url"}
	}
	fallback = strings.Split(fallback, "?")[0]
	return Media{
		URL:      fallback,
		Filename: "reddit_" + id + ".mp4",
		Headers:  map[string]string{"User-Agent": ChromeUA},
	}, nil
}

func redditPostID(u *url.URL) string {
	parts := pathParts(u)
	for i, p := range parts {
		if p == "comments" && i+1 < len(parts) {
			return parts[i+1]
		}
		if p == "video" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
