package s3gw

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	errUnknownUpload    = errors.New("unknown upload id")
	errInvalidPart      = errors.New("invalid part")
	errMalformedXML     = errors.New("malformed xml")
	errInvalidPartOrder = errors.New("invalid part order")
	errEntityTooSmall   = errors.New("part too small")
)

// partBuffer is one uploaded part staged on local disk until CompleteMultipartUpload.
type partBuffer struct {
	Path     string
	Size     int64
	MD5      string
	Modified time.Time
}

type multipartUpload struct {
	Bucket      string
	Key         string
	ContentType string
	Initiated   time.Time
	Dir         string
	mu          sync.Mutex
	parts       map[int]*partBuffer
}

type multipartTracker struct {
	dataDir string
	mu      sync.Mutex
	uploads map[string]*multipartUpload
}

func newMultipartTracker(dataDir string) *multipartTracker {
	t := &multipartTracker{dataDir: dataDir, uploads: map[string]*multipartUpload{}}
	_ = os.MkdirAll(t.stagingDir(), 0o700)
	return t
}

func (t *multipartTracker) stagingDir() string {
	return filepath.Join(t.dataDir, "s3-multipart")
}

// shortcut: unfinished uploads expire on restart; persist their metadata when cross-restart resume is required.
func (t *multipartTracker) cleanStale() {
	entries, err := os.ReadDir(t.stagingDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			_ = os.RemoveAll(filepath.Join(t.stagingDir(), e.Name()))
		}
	}
}

func (t *multipartTracker) get(uploadID string) (*multipartUpload, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	up, ok := t.uploads[uploadID]
	if !ok {
		return nil, errUnknownUpload
	}
	return up, nil
}

func (t *multipartTracker) getForObject(uploadID, bucket, key string) (*multipartUpload, error) {
	up, err := t.get(uploadID)
	if err != nil {
		return nil, err
	}
	if up.Bucket != bucket || up.Key != key {
		return nil, errUnknownUpload
	}
	return up, nil
}

func (t *multipartTracker) create(bucket, key, contentType string) (string, error) {
	uploadID := uuid.NewString()
	up := &multipartUpload{
		Bucket:      bucket,
		Key:         key,
		ContentType: contentType,
		Initiated:   time.Now().UTC(),
		Dir:         filepath.Join(t.stagingDir(), uploadID),
		parts:       map[int]*partBuffer{},
	}
	if err := os.MkdirAll(up.Dir, 0o700); err != nil {
		return "", err
	}
	t.mu.Lock()
	t.uploads[uploadID] = up
	t.mu.Unlock()
	return uploadID, nil
}

// putPart streams one part to disk, returning its MD5 (the S3 part ETag).
func (t *multipartTracker) putPart(uploadID string, partNo int, r io.Reader) (string, error) {
	if partNo < 1 || partNo > 10000 {
		return "", fmt.Errorf("%w: part number must be 1..10000", errInvalidPart)
	}
	up, err := t.get(uploadID)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(up.Dir, "part-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	h := md5.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	md5Hex := hex.EncodeToString(h.Sum(nil))
	up.mu.Lock()
	defer up.mu.Unlock()
	path := filepath.Join(up.Dir, fmt.Sprintf("part-%05d", partNo))
	if err := os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	up.parts[partNo] = &partBuffer{Path: path, Size: n, MD5: md5Hex, Modified: time.Now().UTC()}
	return md5Hex, nil
}

// assemble validates the requested parts and returns a reader over their
// concatenation plus a cleanup function.
func (t *multipartTracker) assemble(uploadID string, requested []completePartRequest) (io.Reader, func(), error) {
	up, err := t.get(uploadID)
	if err != nil {
		return nil, nil, err
	}
	if len(requested) == 0 {
		return nil, nil, fmt.Errorf("%w: no parts in complete request", errMalformedXML)
	}
	up.mu.Lock()
	defer up.mu.Unlock()
	ordered := make([]int, 0, len(requested))
	previous := 0
	for i, p := range requested {
		if p.PartNumber <= previous {
			return nil, nil, errInvalidPartOrder
		}
		previous = p.PartNumber
		buf, ok := up.parts[p.PartNumber]
		if !ok {
			return nil, nil, fmt.Errorf("%w: part %d not uploaded", errInvalidPart, p.PartNumber)
		}
		if strings.Trim(p.ETag, `"`) != buf.MD5 {
			return nil, nil, fmt.Errorf("%w: part %d etag mismatch", errInvalidPart, p.PartNumber)
		}
		if i < len(requested)-1 && buf.Size < 5<<20 {
			return nil, nil, errEntityTooSmall
		}
		ordered = append(ordered, p.PartNumber)
	}
	readers := make([]io.Reader, 0, len(ordered))
	files := make([]*os.File, 0, len(ordered))
	for _, no := range ordered {
		f, err := os.Open(up.parts[no].Path)
		if err != nil {
			for _, opened := range files {
				_ = opened.Close()
			}
			return nil, nil, err
		}
		files = append(files, f)
		readers = append(readers, f)
	}
	cleanup := func() {
		for _, f := range files {
			_ = f.Close()
		}
	}
	return io.MultiReader(readers...), cleanup, nil
}

func (t *multipartTracker) abort(uploadID string) {
	t.mu.Lock()
	up, ok := t.uploads[uploadID]
	if ok {
		delete(t.uploads, uploadID)
	}
	t.mu.Unlock()
	if up != nil {
		_ = os.RemoveAll(up.Dir)
	}
}
