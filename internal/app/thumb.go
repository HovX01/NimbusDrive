package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vrc/nimbus/internal/domain"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const thumbMax = 480

func (s *Services) thumbPath(id string) string {
	return filepath.Join(s.DataDir, "thumbs", id+".jpg")
}

func (s *Services) removeThumb(id string) {
	_ = os.Remove(s.thumbPath(id))
}

// EnsureThumb builds a small JPEG preview on disk (images + video posters).
func (s *Services) EnsureThumb(ctx context.Context, id string) error {
	path := s.thumbPath(id)
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return nil
	}
	node, err := s.Nodes.Get(ctx, id)
	if err != nil {
		return err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		return domain.ErrNotFound
	}

	switch {
	case strings.HasPrefix(node.MimeType, "image/") || isImageFilename(node.Name):
		return s.ensureImageThumb(ctx, id, path)
	case strings.HasPrefix(node.MimeType, "video/") || isVideoFilename(node.Name):
		return s.ensureVideoThumb(ctx, node, path)
	default:
		return domain.ErrNotFound
	}
}

func (s *Services) ensureImageThumb(ctx context.Context, id, path string) error {
	var raw bytes.Buffer
	if _, err := s.Download(ctx, id, &raw); err != nil {
		return err
	}
	img, _, err := image.Decode(bytes.NewReader(raw.Bytes()))
	if err != nil {
		return fmt.Errorf("%w: decode image", domain.ErrValidation)
	}
	return writeThumbJPEG(path, resizeMax(img, thumbMax))
}

func (s *Services) ensureVideoThumb(ctx context.Context, node domain.Node, path string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("%w: ffmpeg not installed", domain.ErrNotConfigured)
	}

	parts, err := s.Parts.ListByFile(ctx, node.ID)
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return domain.ErrNotFound
	}

	src, err := s.ensureMediaCache(ctx, node, parts)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp.jpg"
	_ = os.Remove(tmp)

	// Grab a frame ~1s in (or start); scale to thumb width.
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-ss", "1",
		"-i", src,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-2", thumbMax),
		"-q:v", "4",
		tmp,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Short clips / bad seek — retry from the start.
		cmd = exec.CommandContext(ctx, "ffmpeg",
			"-y",
			"-i", src,
			"-frames:v", "1",
			"-vf", fmt.Sprintf("scale=%d:-2", thumbMax),
			"-q:v", "4",
			tmp,
		)
		out2, err2 := cmd.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("ffmpeg thumb: %w (%s)", err2, truncateOut(out)+" "+truncateOut(out2))
		}
	}
	if st, err := os.Stat(tmp); err != nil || st.Size() == 0 {
		_ = os.Remove(tmp)
		return fmt.Errorf("ffmpeg produced empty thumb")
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func writeThumbJPEG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 82}); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func truncateOut(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[len(s)-200:]
	}
	return s
}

// ThumbBytes returns a cached (or freshly built) JPEG thumbnail.
func (s *Services) ThumbBytes(ctx context.Context, id string) ([]byte, error) {
	path := s.thumbPath(id)
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return b, nil
	}
	if err := s.EnsureThumb(ctx, id); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func resizeMax(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	if w <= max && h <= max {
		return src
	}
	scale := float64(max) / float64(w)
	if float64(h)*scale > float64(max) {
		scale = float64(max) / float64(h)
	}
	nw := int(float64(w)*scale + 0.5)
	nh := int(float64(h)*scale + 0.5)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

func isImageFilename(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func isVideoFilename(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".ogv":
		return true
	default:
		return false
	}
}
