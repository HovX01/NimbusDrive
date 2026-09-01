package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// ProgressFunc reports download progress 0–100.
type ProgressFunc func(pct float64, msg string)

// DownloadToFile GETs media.URL with headers into destDir, returning the file path.
// When media.LocalPath is set (VidBee sidecar), the file is copied instead.
func DownloadToFile(ctx context.Context, m Media, destDir string, onProgress ProgressFunc) (string, error) {
	if m.LocalPath != "" {
		return copyLocalFile(m, destDir, onProgress)
	}
	if m.URL == "" {
		return "", Err{Code: CodeFetchEmpty, Message: "empty media url"}
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", err
	}
	name := m.Filename
	if name == "" {
		name = "download.bin"
	}
	dest := filepath.Join(destDir, filepath.Base(name))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return "", Err{Code: CodeFetchFail, Message: err.Error()}
	}
	for k, v := range m.Headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ChromeUA)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", Err{Code: CodeFetchFail, Message: err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return "", Err{Code: CodeFetchFail, Message: fmt.Sprintf("cdn http %d", res.StatusCode)}
	}

	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	total := res.ContentLength
	var written int64
	buf := make([]byte, 64*1024)
	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				_ = os.Remove(dest)
				return "", werr
			}
			written += int64(n)
			if onProgress != nil && total > 0 {
				pct := float64(written) / float64(total) * 100
				onProgress(pct, fmt.Sprintf("Downloading… %.0f%%", pct))
			} else if onProgress != nil && written%(512*1024) < int64(n) {
				onProgress(-1, "Downloading… "+strconv.FormatInt(written/1024, 10)+" KB")
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = os.Remove(dest)
			return "", Err{Code: CodeFetchFail, Message: readErr.Error()}
		}
	}
	if written < 1000 {
		_ = os.Remove(dest)
		return "", Err{Code: CodeFetchEmpty, Message: "download too small (blocked?)"}
	}
	if onProgress != nil {
		onProgress(100, "Download finished")
	}
	return dest, nil
}

func copyLocalFile(m Media, destDir string, onProgress ProgressFunc) (string, error) {
	src := filepath.Clean(m.LocalPath)
	st, err := os.Stat(src)
	if err != nil {
		return "", Err{Code: CodeFetchFail, Message: "vidbee file not found: " + src}
	}
	if st.IsDir() {
		return "", Err{Code: CodeFetchEmpty, Message: "vidbee path is a directory"}
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", err
	}
	name := m.Filename
	if name == "" {
		name = filepath.Base(src)
	}
	dest := filepath.Join(destDir, filepath.Base(name))

	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if onProgress != nil {
		onProgress(10, "Copying from VidBee…")
	}
	written, err := io.Copy(out, in)
	if err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	if written < 1000 {
		_ = os.Remove(dest)
		return "", Err{Code: CodeFetchEmpty, Message: "vidbee file too small"}
	}
	if onProgress != nil {
		onProgress(100, "Download finished")
	}
	return dest, nil
}
