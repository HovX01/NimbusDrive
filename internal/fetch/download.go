package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProgressFunc reports download progress 0–100.
type ProgressFunc func(pct float64, msg string)

// DownloadToFile GETs media.URL with headers into destDir, returning the file path.
// When media.LocalPath is set (VidBee sidecar), the file is copied instead.
// For multi-item Media, prefer DownloadAllToFiles.
func DownloadToFile(ctx context.Context, m Media, destDir string, onProgress ProgressFunc) (string, error) {
	items := m.AllItems()
	if len(items) == 0 {
		return "", Err{Code: CodeFetchEmpty, Message: "empty media url"}
	}
	return downloadItem(ctx, items[0], destDir, onProgress)
}

// DownloadAllToFiles downloads every item (carousel / multi-media posts).
func DownloadAllToFiles(ctx context.Context, m Media, destDir string, onProgress ProgressFunc) ([]string, error) {
	items := m.AllItems()
	if len(items) == 0 {
		return nil, Err{Code: CodeFetchEmpty, Message: "empty media url"}
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	used := map[string]int{}
	for i, item := range items {
		name := item.Filename
		if name == "" {
			name = fmt.Sprintf("download_%d.bin", i+1)
		}
		base := filepath.Base(name)
		if n, ok := used[base]; ok {
			ext := filepath.Ext(base)
			stem := strings.TrimSuffix(base, ext)
			base = fmt.Sprintf("%s_%d%s", stem, n+1, ext)
			used[filepath.Base(name)] = n + 1
		} else {
			used[base] = 1
		}
		item.Filename = base
		if onProgress != nil {
			onProgress(-1, fmt.Sprintf("Downloading %d/%d…", i+1, len(items)))
		}
		path, err := downloadItem(ctx, item, destDir, nil)
		if err != nil {
			for _, p := range out {
				_ = os.Remove(p)
			}
			return nil, err
		}
		out = append(out, path)
		if onProgress != nil {
			pct := float64(i+1) / float64(len(items)) * 100
			onProgress(pct, fmt.Sprintf("Downloaded %d/%d", i+1, len(items)))
		}
	}
	return out, nil
}

func downloadItem(ctx context.Context, item MediaItem, destDir string, onProgress ProgressFunc) (string, error) {
	if item.LocalPath != "" {
		return copyLocalItem(item, destDir, onProgress)
	}
	if item.URL == "" {
		return "", Err{Code: CodeFetchEmpty, Message: "empty media url"}
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", err
	}
	name := item.Filename
	if name == "" {
		name = "download.bin"
	}
	dest := filepath.Join(destDir, filepath.Base(name))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return "", Err{Code: CodeFetchFail, Message: err.Error()}
	}
	for k, v := range item.Headers {
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

	// Prefer Content-Type when Cobalt/CDN labels Live Photo motion as a still (.jpg).
	name, dest = maybeRenameByExt(name, destDir, extFromContentType(res.Header.Get("Content-Type")))

	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}

	total := res.ContentLength
	var written int64
	var head []byte
	buf := make([]byte, 64*1024)
	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			if len(head) < 64 {
				need := 64 - len(head)
				if need > n {
					need = n
				}
				head = append(head, buf[:need]...)
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				_ = f.Close()
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
			_ = f.Close()
			_ = os.Remove(dest)
			return "", Err{Code: CodeFetchFail, Message: readErr.Error()}
		}
	}
	_ = f.Close()

	// Magic-byte sniff: Live Photo clips are often mp4 bytes saved as .jpg.
	if sniffed := sniffMediaExt(head); sniffed != "" {
		fixed, err := renameIfExtMismatch(dest, name, sniffed)
		if err != nil {
			_ = os.Remove(dest)
			return "", err
		}
		dest, name = fixed, filepath.Base(fixed)
	}

	minBytes := int64(1000)
	if isLikelyImageName(name) {
		minBytes = 100
	}
	if written < minBytes {
		_ = os.Remove(dest)
		return "", Err{Code: CodeFetchEmpty, Message: "download too small (blocked?)"}
	}
	if onProgress != nil {
		onProgress(100, "Download finished")
	}
	return dest, nil
}

func isLikelyImageName(name string) bool {
	return isLikelyImageExt(strings.ToLower(filepath.Ext(name)))
}

func isLikelyImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".heic":
		return true
	default:
		return false
	}
}

func isLikelyVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".webm", ".mov", ".m4v", ".mkv":
		return true
	default:
		return false
	}
}

func maybeRenameByExt(name, destDir, wantExt string) (string, string) {
	if wantExt == "" {
		return name, filepath.Join(destDir, filepath.Base(name))
	}
	cur := strings.ToLower(filepath.Ext(name))
	mismatch := cur == "" || cur == ".bin" || cur == ".tmp" ||
		(isLikelyImageExt(wantExt) && !isLikelyImageExt(cur)) ||
		(isLikelyVideoExt(wantExt) && isLikelyImageExt(cur)) ||
		(isLikelyVideoExt(wantExt) && cur != wantExt && !isLikelyVideoExt(cur))
	if !mismatch {
		return name, filepath.Join(destDir, filepath.Base(name))
	}
	stem := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if stem == "" {
		stem = "download"
	}
	// Live Photo motion often arrives labeled as photo_N.jpg → photo_N_live.mp4
	if isLikelyVideoExt(wantExt) && isLikelyImageExt(cur) && !strings.Contains(strings.ToLower(stem), "live") {
		stem = stem + "_live"
	}
	name = stem + wantExt
	return name, filepath.Join(destDir, name)
}

func renameIfExtMismatch(dest, name, wantExt string) (string, error) {
	cur := strings.ToLower(filepath.Ext(name))
	if cur == wantExt {
		return dest, nil
	}
	mismatch := cur == "" || cur == ".bin" || cur == ".tmp" ||
		(isLikelyImageExt(wantExt) && !isLikelyImageExt(cur)) ||
		(isLikelyVideoExt(wantExt) && isLikelyImageExt(cur)) ||
		(isLikelyVideoExt(wantExt) && !isLikelyVideoExt(cur))
	if !mismatch {
		return dest, nil
	}
	stem := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if stem == "" {
		stem = "download"
	}
	if isLikelyVideoExt(wantExt) && isLikelyImageExt(cur) && !strings.Contains(strings.ToLower(stem), "live") {
		stem = stem + "_live"
	}
	next := filepath.Join(filepath.Dir(dest), stem+wantExt)
	if err := os.Rename(dest, next); err != nil {
		return "", err
	}
	return next, nil
}

// sniffMediaExt detects real type from file header (Live Photos are often mp4).
func sniffMediaExt(head []byte) string {
	if len(head) < 12 {
		return ""
	}
	if head[0] == 0xff && head[1] == 0xd8 && head[2] == 0xff {
		return ".jpg"
	}
	if head[0] == 0x89 && string(head[1:4]) == "PNG" {
		return ".png"
	}
	if string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP" {
		return ".webp"
	}
	if string(head[0:4]) == "GIF8" {
		return ".gif"
	}
	// ISO BMFF: ....ftyp????
	if string(head[4:8]) == "ftyp" {
		brand := string(head[8:12])
		switch {
		case strings.HasPrefix(brand, "heic"), strings.HasPrefix(brand, "heif"), strings.HasPrefix(brand, "mif1"):
			return ".heic"
		case strings.HasPrefix(brand, "qt"):
			return ".mov"
		default:
			return ".mp4"
		}
	}
	if string(head[0:4]) == "\x1aE\xdf\xa3" {
		return ".webm"
	}
	return ""
}

func copyLocalFile(m Media, destDir string, onProgress ProgressFunc) (string, error) {
	return copyLocalItem(MediaItem{LocalPath: m.LocalPath, Filename: m.Filename}, destDir, onProgress)
}

func copyLocalItem(item MediaItem, destDir string, onProgress ProgressFunc) (string, error) {
	src := filepath.Clean(item.LocalPath)
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
	name := item.Filename
	if name == "" {
		name = filepath.Base(src)
	}
	dest := filepath.Join(destDir, filepath.Base(name))
	if samePath(src, dest) {
		if onProgress != nil {
			onProgress(100, "Download finished")
		}
		return dest, nil
	}

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
	if written < 100 {
		_ = os.Remove(dest)
		return "", Err{Code: CodeFetchEmpty, Message: "vidbee file too small"}
	}
	if onProgress != nil {
		onProgress(100, "Download finished")
	}
	return dest, nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
