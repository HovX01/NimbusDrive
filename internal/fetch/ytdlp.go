package fetch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func resolveViaYtDLP(ctx context.Context, pageURL string, opt Options) (Media, error) {
	bin := ytDLPBinary()
	if bin == "" {
		return Media{}, Err{Code: CodeLinkUnsupported, Message: "yt-dlp not installed or disabled"}
	}
	workDir := strings.TrimSpace(opt.TempDir)
	if workDir == "" {
		var err error
		workDir, err = os.MkdirTemp("", "nimbus-ytdlp-*")
		if err != nil {
			return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
		}
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}

	args := []string{
		"--no-progress",
		"--newline",
		"--no-part",
		"--restrict-filenames",
		"--windows-filenames",
		"--user-agent", ChromeUA,
		"-o", filepath.Join(workDir, "%(extractor)s_%(id)s_%(playlist_index|)s.%(ext)s"),
	}
	if cookies := strings.TrimSpace(opt.Cookies); cookies != "" {
		args = append(args, "--cookies", cookies)
	}
	if !ytDLPAllowPlaylist() {
		args = append(args, "--no-playlist")
	} else if max := ytDLPMaxDownloads(); max > 0 {
		args = append(args, "--max-downloads", strconv.Itoa(max))
	}
	if opt.Mode == "audio" {
		args = append(args, "-x", "--audio-format", "mp3", "-f", "bestaudio/best")
	} else {
		args = append(args, "--merge-output-format", "mp4", "-f", ytDLPFormat(opt.MaxHeight))
	}
	args = append(args, pageURL)

	if opt.OnProgress != nil {
		opt.OnProgress(-1, "Downloading via yt-dlp...")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return Media{}, Err{Code: CodeFetchFail, Message: "yt-dlp failed: " + tailText(msg, 800)}
	}
	paths, err := downloadedFiles(workDir)
	if err != nil {
		return Media{}, err
	}
	items := make([]MediaItem, 0, len(paths))
	for _, p := range paths {
		items = append(items, MediaItem{LocalPath: p, Filename: filepath.Base(p)})
	}
	if opt.OnProgress != nil {
		opt.OnProgress(100, "yt-dlp download finished")
	}
	return Media{
		LocalPath: items[0].LocalPath,
		Filename:  items[0].Filename,
		Service:   "yt-dlp",
		Items:     items,
	}, nil
}

func ytDLPBinary() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("NIMBUS_YTDLP_DISABLED")), "true") {
		return ""
	}
	candidates := []string{
		strings.TrimSpace(os.Getenv("NIMBUS_YTDLP_PATH")),
		"yt-dlp",
		"yt-dlp.exe",
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

func ytDLPConfigured() bool {
	return ytDLPBinary() != ""
}

func ytDLPFormat(maxHeight int) string {
	h := SnapHeight(maxHeight)
	if h <= 0 || h >= 4320 {
		return "bv*+ba/best"
	}
	return fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]/best", h, h)
}

func ytDLPAllowPlaylist() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("NIMBUS_YTDLP_ALLOW_PLAYLIST")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func ytDLPMaxDownloads() int {
	v := strings.TrimSpace(os.Getenv("NIMBUS_YTDLP_MAX_DOWNLOADS"))
	if v == "" {
		return 50
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 50
	}
	return n
}

func downloadedFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".ytdl") || strings.HasSuffix(name, ".temp") {
			continue
		}
		p := filepath.Join(dir, name)
		st, err := os.Stat(p)
		if err == nil && st.Size() > 100 {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, Err{Code: CodeFetchEmpty, Message: "yt-dlp produced no media files"}
	}
	return paths, nil
}

func tailText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}
