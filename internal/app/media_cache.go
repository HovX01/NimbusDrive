package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/vrc/nimbus/internal/domain"
	"golang.org/x/sync/singleflight"
)

// mediaCacheGroup dedupes Telegram → disk pulls so parallel Range requests
// (browser video seeking) don't each re-download the same file.
var mediaCacheGroup singleflight.Group

func (s *Services) mediaCachePath(id string) string {
	return filepath.Join(s.DataDir, "mediacache", id)
}

func (s *Services) removeMediaCache(id string) {
	_ = os.Remove(s.mediaCachePath(id))
	matches, _ := filepath.Glob(s.mediaCachePath(id) + ".tmp.*")
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// ensureMediaCache downloads the file from Telegram once onto local disk.
// Browsers issue many small Range requests; without this cache each seek
// re-fetches whole Telegram parts and playback becomes laggy / flaky.
func (s *Services) ensureMediaCache(ctx context.Context, node domain.Node, parts []domain.FilePart) (string, error) {
	path := s.mediaCachePath(node.ID)
	var total int64
	for _, p := range parts {
		total += p.Size
	}
	if st, err := os.Stat(path); err == nil && st.Size() == total && total > 0 {
		return path, nil
	}

	v, err, _ := mediaCacheGroup.Do(node.ID, func() (any, error) {
		var total int64
		for _, p := range parts {
			total += p.Size
		}
		if st, err := os.Stat(path); err == nil && st.Size() == total && total > 0 {
			return path, nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return "", err
		}
		tmp := path + ".tmp." + strconv.Itoa(os.Getpid())
		_ = os.Remove(tmp)
		f, err := os.Create(tmp)
		if err != nil {
			return "", err
		}
		for _, part := range parts {
			if err := s.Blobs.DownloadPart(ctx, part.ChannelID, part.MessageID, f); err != nil {
				_ = f.Close()
				_ = os.Remove(tmp)
				return "", fmt.Errorf("telegram download: %w", err)
			}
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
		st, err := os.Stat(tmp)
		if err != nil || st.Size() == 0 {
			_ = os.Remove(tmp)
			return "", fmt.Errorf("empty media cache")
		}
		_ = os.Remove(path)
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
		return path, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (s *Services) serveLocalRange(path string, w io.Writer, start, endInclusive int64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}
	_, err = io.CopyN(w, f, endInclusive-start+1)
	return err
}
