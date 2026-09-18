package app

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

const maxImportBytes = 500 << 20 // 500 MiB

// ImportURL downloads a remote file and stores it like a normal upload.
func (s *Services) ImportURL(ctx context.Context, parentID, rawURL, name string) (domain.Node, error) {
	u, err := validateImportURL(rawURL)
	if err != nil {
		return domain.Node{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return domain.Node{}, fmt.Errorf("%w: %v", domain.ErrValidation, err)
	}
	req.Header.Set("User-Agent", "nimbus/1.0")

	client := &http.Client{
		Timeout: 30 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("%w: too many redirects", domain.ErrValidation)
			}
			_, err := validateImportURL(req.URL.String())
			return err
		},
	}

	res, err := client.Do(req)
	if err != nil {
		return domain.Node{}, fmt.Errorf("%w: download failed: %v", domain.ErrValidation, err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return domain.Node{}, fmt.Errorf("%w: remote returned %d", domain.ErrValidation, res.StatusCode)
	}
	if res.ContentLength > maxImportBytes {
		return domain.Node{}, fmt.Errorf("%w: file too large (max %d bytes)", domain.ErrValidation, maxImportBytes)
	}

	filename := strings.TrimSpace(name)
	if filename == "" {
		filename = filenameFromResponse(u, res)
	}

	mimeType := res.Header.Get("Content-Type")
	if mimeType != "" {
		mimeType, _, _ = mime.ParseMediaType(mimeType)
	}

	var r io.Reader = res.Body
	if res.ContentLength > 0 {
		r = io.LimitReader(res.Body, res.ContentLength)
	} else {
		r = io.LimitReader(res.Body, maxImportBytes+1)
	}

	node, err := s.Upload(ctx, parentID, filename, mimeType, r, res.ContentLength)
	if err != nil {
		return domain.Node{}, err
	}
	if node.Size > maxImportBytes {
		_ = s.Delete(ctx, node.ID)
		return domain.Node{}, fmt.Errorf("%w: file too large (max %d bytes)", domain.ErrValidation, maxImportBytes)
	}
	return node, nil
}

func validateImportURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid url", domain.ErrValidation)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: url must be http or https", domain.ErrValidation)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: url must not contain credentials", domain.ErrValidation)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%w: invalid url", domain.ErrValidation)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("%w: private addresses not allowed", domain.ErrValidation)
		}
		return u, nil
	}
	h := strings.ToLower(host)
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || h == "metadata.google.internal" {
		return nil, fmt.Errorf("%w: host not allowed", domain.ErrValidation)
	}
	return u, nil
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	return ip.Equal(net.ParseIP("169.254.169.254"))
}

func filenameFromResponse(u *url.URL, res *http.Response) string {
	if cd := res.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if fn := params["filename"]; fn != "" {
				return fn
			}
		}
	}
	base := filepath.Base(u.Path)
	if base != "" && base != "." && base != "/" {
		return base
	}
	ext := ""
	if ct := res.Header.Get("Content-Type"); ct != "" {
		if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
			ext = exts[0]
		}
	}
	return "import" + ext
}
