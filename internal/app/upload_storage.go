package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

var ErrUploadSpace = errors.New("insufficient temporary disk space for upload")

const uploadDiskReserve = 256 << 20

var uploadReservations = struct {
	sync.Mutex
	bytes map[uint64]int64
}{bytes: make(map[uint64]int64)}

// ReserveUpload bounds staging on the filesystem shared by concurrent requests.
func ReserveUpload(path string, size int64) (int64, func(), error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, nil, err
	}
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return 0, nil, err
	}
	return reserveUpload(uint64(stat.Dev), int64(fs.Bavail)*int64(fs.Bsize), size)
}

func reserveUpload(device uint64, free, size int64) (int64, func(), error) {
	uploadReservations.Lock()
	defer uploadReservations.Unlock()
	// shortcut: reservations include bytes already written; track outstanding bytes if this rejects valid concurrent uploads.
	available := free - uploadDiskReserve - uploadReservations.bytes[device]
	if available <= 0 || size > available {
		return 0, nil, ErrUploadSpace
	}
	if size <= 0 {
		size = available
	}
	uploadReservations.bytes[device] += size
	var once sync.Once
	return size, func() {
		once.Do(func() {
			uploadReservations.Lock()
			defer uploadReservations.Unlock()
			uploadReservations.bytes[device] -= size
			if uploadReservations.bytes[device] == 0 {
				delete(uploadReservations.bytes, device)
			}
		})
	}, nil
}

type uploadContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r uploadContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func prepareUpload(ctx context.Context, r io.Reader, size int64, dir string) (reader io.ReadSeeker, hash string, cleanup func(), err error) {
	cleanup = func() {}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	h := sha256.New()
	var dst io.Writer = h
	var start, limit int64
	if seeker, ok := r.(io.ReadSeeker); ok {
		reader = seeker
		start, err = seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, "", cleanup, err
		}
	} else {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, "", cleanup, err
		}
		var release func()
		limit, release, err = ReserveUpload(dir, size)
		if err != nil {
			return nil, "", cleanup, err
		}
		cleanup = release
		tmp, err := os.CreateTemp(dir, "up-*")
		if err != nil {
			return nil, "", cleanup, err
		}
		cleanup = func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			release()
		}
		reader = tmp
		dst = io.MultiWriter(tmp, h)
		r = io.LimitReader(r, limit+1)
	}
	written, err := io.Copy(dst, uploadContextReader{ctx: ctx, r: r})
	if err != nil {
		return nil, "", cleanup, err
	}
	if written == 0 || (size > 0 && written != size) {
		return nil, "", cleanup, fmt.Errorf("%w: empty file or upload size mismatch", domain.ErrValidation)
	}
	if limit > 0 && written > limit {
		return nil, "", cleanup, ErrUploadSpace
	}
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return nil, "", cleanup, err
	}
	return reader, hex.EncodeToString(h.Sum(nil)), cleanup, nil
}

func cleanupUpload(ctx context.Context, blobs domain.BlobStore, parts domain.PartRepository, nodes domain.NodeRepository, fileID string, uploaded []domain.FilePart) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	ids := make([]int, 0, len(uploaded))
	var channelID int64
	for _, part := range uploaded {
		ids = append(ids, part.MessageID)
		channelID = part.ChannelID
	}
	// Save recovery IDs before remote cleanup can exhaust its deadline.
	persistErr := parts.ReplaceParts(ctx, fileID, uploaded)
	if err := blobs.DeleteMessages(ctx, channelID, ids); err != nil {
		return errors.Join(err, persistErr)
	}
	if err := parts.DeleteByFile(ctx, fileID); err != nil {
		return err
	}
	return nodes.SoftDelete(ctx, fileID)
}
