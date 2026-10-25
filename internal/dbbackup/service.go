package dbbackup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
)

const defaultPrefix = "nimbus-db-backup"

// DatabaseSource creates a consistent SQLite snapshot at path.
type DatabaseSource interface {
	SnapshotToFile(ctx context.Context, path string) error
}

// Entry is one database backup stored in the Telegram channel.
type Entry struct {
	Name      string    `json:"name"`
	MessageID int       `json:"message_id"`
	ChannelID int64     `json:"channel_id"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type indexFile struct {
	Backups []Entry `json:"backups"`
}

// Service snapshots the metadata database and uploads it as one named
// Telegram document. Chunk data remains opaque; database backups are
// deliberately named so they are easy to find in the channel.
type Service struct {
	dataDir string
	db      DatabaseSource
	blobs   domain.BlobStore
	prefix  string
	keep    int
	now     func() time.Time

	mu sync.Mutex
}

func NewService(dataDir string, db DatabaseSource, blobs domain.BlobStore, prefix string, keep int) *Service {
	if keep < 0 {
		keep = 0
	}
	return &Service{
		dataDir: dataDir,
		db:      db,
		blobs:   blobs,
		prefix:  sanitizePrefix(prefix),
		keep:    keep,
		now:     time.Now,
	}
}

func (s *Service) Enabled() bool {
	return s != nil && s.db != nil && s.blobs != nil
}

// RunOnce creates a snapshot, uploads it, records it in the local index, and
// prunes old backups when keep > 0. It is safe to call concurrently.
func (s *Service) RunOnce(ctx context.Context) (Entry, error) {
	if !s.Enabled() {
		return Entry{}, fmt.Errorf("%w: telegram database backup not configured", domain.ErrNotConfigured)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	channelID, err := s.blobs.EnsureStorageChannel(ctx)
	if err != nil {
		return Entry{}, fmt.Errorf("ensure storage channel: %w", err)
	}
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return Entry{}, fmt.Errorf("backup dir: %w", err)
	}
	tmpPath := filepath.Join(s.dataDir, "db-backup-"+uuid.NewString()+".sqlite")
	defer os.Remove(tmpPath)

	if err := s.db.SnapshotToFile(ctx, tmpPath); err != nil {
		return Entry{}, fmt.Errorf("snapshot database: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return Entry{}, fmt.Errorf("chmod snapshot: %w", err)
	}
	sum, size, err := hashFile(tmpPath)
	if err != nil {
		return Entry{}, err
	}

	created := s.now().UTC()
	suffix, err := randomSuffix()
	if err != nil {
		return Entry{}, err
	}
	name := fmt.Sprintf("%s-%s-%s.sqlite", s.prefix, created.Format("20060102T150405Z"), suffix)
	caption := fmt.Sprintf("Nimbus database backup | %s | %d bytes | sha256:%s", created.Format(time.RFC3339), size, sum)

	f, err := os.Open(tmpPath)
	if err != nil {
		return Entry{}, fmt.Errorf("open snapshot: %w", err)
	}
	defer f.Close()
	messageID, err := s.blobs.UploadDocument(ctx, channelID, name, caption, f, size)
	if err != nil {
		return Entry{}, fmt.Errorf("upload database backup: %w", err)
	}

	entry := Entry{
		Name:      name,
		MessageID: messageID,
		ChannelID: channelID,
		Size:      size,
		SHA256:    sum,
		CreatedAt: created,
	}
	if err := s.record(entry); err != nil {
		log.Printf("db backup: index update failed for %s: %v", name, err)
	}
	s.prune(ctx)
	return entry, nil
}

// List returns the local backup index, oldest first. The index is a
// convenience for pruning and verification; the Telegram messages remain the
// source of truth if it is lost.
func (s *Service) List() ([]Entry, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: telegram database backup not configured", domain.ErrNotConfigured)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	if idx.Backups == nil {
		return []Entry{}, nil
	}
	return append([]Entry(nil), idx.Backups...), nil
}

func (s *Service) record(entry Entry) error {
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	idx.Backups = append(idx.Backups, entry)
	return s.saveIndex(idx)
}

func (s *Service) prune(ctx context.Context) {
	if s.keep <= 0 {
		return
	}
	idx, err := s.loadIndex()
	if err != nil {
		log.Printf("db backup: load index for prune: %v", err)
		return
	}
	sort.Slice(idx.Backups, func(i, j int) bool {
		return idx.Backups[i].CreatedAt.Before(idx.Backups[j].CreatedAt)
	})
	changed := false
	for len(idx.Backups) > s.keep {
		oldest := idx.Backups[0]
		if err := s.blobs.DeleteMessages(ctx, oldest.ChannelID, []int{oldest.MessageID}); err != nil {
			log.Printf("db backup: prune %s: %v", oldest.Name, err)
			break
		}
		log.Printf("db backup: pruned %s (message %d)", oldest.Name, oldest.MessageID)
		idx.Backups = idx.Backups[1:]
		changed = true
	}
	if changed {
		if err := s.saveIndex(idx); err != nil {
			log.Printf("db backup: save pruned index: %v", err)
		}
	}
}

func (s *Service) indexPath() string {
	return filepath.Join(s.dataDir, "db-backup-index.json")
}

func (s *Service) loadIndex() (indexFile, error) {
	var idx indexFile
	b, err := os.ReadFile(s.indexPath())
	if os.IsNotExist(err) {
		return idx, nil
	}
	if err != nil {
		return idx, err
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		return idx, err
	}
	return idx, nil
}

func (s *Service) saveIndex(idx indexFile) error {
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.indexPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.indexPath())
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func randomSuffix() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("random backup suffix: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func sanitizePrefix(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return defaultPrefix
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}
