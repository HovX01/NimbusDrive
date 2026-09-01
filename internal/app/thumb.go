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

// EnsureThumb builds a small JPEG preview on disk (S3/Drive-style CDN thumb cache).
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
	if !strings.HasPrefix(node.MimeType, "image/") && !isImageFilename(node.Name) {
		return domain.ErrNotFound
	}

	var raw bytes.Buffer
	if _, err := s.Download(ctx, id, &raw); err != nil {
		return err
	}
	img, _, err := image.Decode(bytes.NewReader(raw.Bytes()))
	if err != nil {
		return fmt.Errorf("%w: decode image", domain.ErrValidation)
	}
	resized := resizeMax(img, thumbMax)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := jpeg.Encode(f, resized, &jpeg.Options{Quality: 82}); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
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
