package s3gw

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/google/uuid"
)

var (
	errUnknownUpload = errors.New("unknown upload id")
	errInvalidPart   = errors.New("invalid part")
	errMalformedXML  = errors.New("malformed xml")
)

// partBuffer is one uploaded part staged on local disk until CompleteMultipartUpload.
type partBuffer struct {
	Path string
	Size int64
	MD5  string
}

type multipartUpload struct {
	Bucket      string
	Key         string
	ContentType string
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

// cleanStale removes part staging dirs left over from a previous process.
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

func (t *multipartTracker) create(bucket, key, contentType string) (string, error) {
	uploadID := uuid.NewString()
	up := &multipartUpload{
		Bucket:      bucket,
		Key:         key,
		ContentType: contentType,
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
	path := filepath.Join(up.Dir, fmt.Sprintf("part-%05d", partNo))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	up.mu.Lock()
	up.parts[partNo] = &partBuffer{Path: path, Size: n, MD5: hex.EncodeToString(h.Sum(nil))}
	up.mu.Unlock()
	return up.parts[partNo].MD5, nil
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
	ordered := make([]int, 0, len(requested))
	seen := map[int]bool{}
	for _, p := range requested {
		buf, ok := up.parts[p.PartNumber]
		if !ok {
			return nil, nil, fmt.Errorf("%w: part %d not uploaded", errInvalidPart, p.PartNumber)
		}
		if p.ETag != "" && p.ETag != buf.MD5 {
			return nil, nil, fmt.Errorf("%w: part %d etag mismatch", errInvalidPart, p.PartNumber)
		}
		if seen[p.PartNumber] {
			return nil, nil, fmt.Errorf("%w: duplicate part %d", errInvalidPart, p.PartNumber)
		}
		seen[p.PartNumber] = true
		ordered = append(ordered, p.PartNumber)
	}
	sort.Ints(ordered)
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
		_ = os.RemoveAll(up.Dir)
		t.mu.Lock()
		delete(t.uploads, uploadID)
		t.mu.Unlock()
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
