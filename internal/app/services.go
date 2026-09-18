package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
	"golang.org/x/sync/errgroup"
)

type Services struct {
	Nodes     domain.NodeRepository
	Parts     domain.PartRepository
	Shares    domain.ShareRepository
	Data      domain.DataRepository
	Edits     domain.EditProjectRepository
	BotGrants domain.BotGrantRepository
	Social    domain.SocialConnectionRepository
	SocialApp domain.SocialOAuthConfigRepository
	Blobs     domain.BlobStore
	TG        domain.TelegramAuth
	Messenger domain.TelegramMessenger
	Setup     domain.TelegramSetup
	JWTSecret []byte
	JWTTTL    time.Duration
	ChunkSize int
	DataDir   string
	// UploadWorkers caps parallel Telegram part uploads (enterprise-style pipeline).
	UploadWorkers int

	fetchOnce sync.Once
	fetchJobs *fetchHub

	socialMu      sync.Mutex
	socialStates  map[string]socialOAuthState
	socialBaseURL string

	mediaOnce sync.Once
	mediaJobs *mediaHub

	editExportOnce sync.Once
	editExportJobs *editExportHub

	proxyOnce sync.Once
	proxyJobs *proxyHub
}

type tokenClaims struct {
	TelegramID int64  `json:"tid"`
	Username   string `json:"usr"`
	jwt.RegisteredClaims
}

func (s *Services) SetupStatus(ctx context.Context) (configured bool, authorized bool, err error) {
	configured = s.Setup != nil && s.Setup.IsConfigured()
	if !configured {
		return false, false, nil
	}
	authorized, err = s.TG.IsAuthorized(ctx)
	return configured, authorized, err
}

func (s *Services) ConfigureTelegram(ctx context.Context, apiID int, apiHash string) error {
	if s.Setup == nil {
		return domain.ErrNotConfigured
	}
	return s.Setup.Configure(ctx, apiID, apiHash)
}

func (s *Services) SignIn(ctx context.Context, phone, code, phoneCodeHash, password string) (domain.UserProfile, string, error) {
	if code == "" {
		return domain.UserProfile{}, "", fmt.Errorf("%w: code required", domain.ErrValidation)
	}
	profile, err := s.TG.SignIn(ctx, phone, code, phoneCodeHash, password)
	if err != nil {
		return domain.UserProfile{}, "", err
	}
	token, err := s.issueToken(profile)
	if err != nil {
		return domain.UserProfile{}, "", err
	}
	if _, err := s.Nodes.EnsureRoot(ctx); err != nil {
		return domain.UserProfile{}, "", err
	}
	return profile, token, nil
}

// ResumeSession issues a JWT from an existing Telegram session — no OTP.
func (s *Services) ResumeSession(ctx context.Context) (domain.UserProfile, string, error) {
	ok, err := s.TG.IsAuthorized(ctx)
	if err != nil {
		return domain.UserProfile{}, "", err
	}
	if !ok {
		return domain.UserProfile{}, "", domain.ErrNotAuthenticated
	}
	profile, err := s.TG.CurrentUser(ctx)
	if err != nil {
		return domain.UserProfile{}, "", err
	}
	token, err := s.issueToken(profile)
	if err != nil {
		return domain.UserProfile{}, "", err
	}
	if _, err := s.Nodes.EnsureRoot(ctx); err != nil {
		return domain.UserProfile{}, "", err
	}
	return profile, token, nil
}

func (s *Services) SendCode(ctx context.Context, phone string) (string, error) {
	if phone == "" {
		return "", fmt.Errorf("%w: phone required", domain.ErrValidation)
	}
	// Never request another OTP if already logged into Telegram.
	if ok, err := s.TG.IsAuthorized(ctx); err == nil && ok {
		return "", fmt.Errorf("%w: already signed in — use resume instead of requesting a new code", domain.ErrConflict)
	}
	return s.TG.SendCode(ctx, phone)
}

func (s *Services) Me(ctx context.Context) (domain.UserProfile, error) {
	return s.TG.CurrentUser(ctx)
}

func (s *Services) Avatar(ctx context.Context, w io.Writer) error {
	return s.TG.DownloadAvatar(ctx, w)
}

func (s *Services) Logout(ctx context.Context) error {
	return s.TG.Logout(ctx)
}

func (s *Services) Authorized(ctx context.Context) (bool, error) {
	return s.TG.IsAuthorized(ctx)
}

func (s *Services) issueToken(p domain.UserProfile) (string, error) {
	claims := tokenClaims{
		TelegramID: p.TelegramID,
		Username:   p.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.JWTTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   fmt.Sprintf("%d", p.TelegramID),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(s.JWTSecret)
}

func (s *Services) ParseToken(token string) (tokenClaims, error) {
	parsed, err := jwt.ParseWithClaims(token, &tokenClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, domain.ErrUnauthorized
		}
		return s.JWTSecret, nil
	})
	if err != nil {
		return tokenClaims{}, domain.ErrUnauthorized
	}
	claims, ok := parsed.Claims.(*tokenClaims)
	if !ok || !parsed.Valid {
		return tokenClaims{}, domain.ErrUnauthorized
	}
	return *claims, nil
}

func (s *Services) List(ctx context.Context, parentID string) ([]domain.Node, error) {
	if parentID == "" {
		parentID = "root"
	}
	if _, err := s.Nodes.Get(ctx, parentID); err != nil {
		return nil, err
	}
	return s.Nodes.ListChildren(ctx, parentID)
}

func (s *Services) ListAll(ctx context.Context, limit int) ([]domain.Node, error) {
	return s.Nodes.ListReady(ctx, limit)
}

func (s *Services) Search(ctx context.Context, query string, limit int) ([]domain.SearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	return s.Nodes.Search(ctx, query, limit)
}

func (s *Services) Mkdir(ctx context.Context, parentID, name string) (domain.Node, error) {
	if err := domain.ValidateNodeName(name); err != nil {
		return domain.Node{}, err
	}
	if parentID == "" {
		parentID = "root"
	}
	if _, err := s.Nodes.Get(ctx, parentID); err != nil {
		return domain.Node{}, err
	}
	pid := parentID
	return s.Nodes.Create(ctx, domain.Node{
		ParentID: &pid,
		Name:     name,
		Type:     domain.NodeFolder,
		MimeType: "inode/directory",
		Status:   domain.StatusReady,
	})
}

func (s *Services) Upload(ctx context.Context, parentID, filename string, mimeType string, r io.Reader, _ int64) (domain.Node, error) {
	filename = domain.SanitizeDownloadName(filename)
	if err := domain.ValidateNodeName(filename); err != nil {
		return domain.Node{}, err
	}
	if parentID == "" {
		parentID = "root"
	}
	if _, err := s.Nodes.Get(ctx, parentID); err != nil {
		return domain.Node{}, err
	}
	var nameErr error
	filename, nameErr = s.uniqueChildName(ctx, parentID, filename)
	if nameErr != nil {
		return domain.Node{}, nameErr
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		if guessed := mime.TypeByExtension(filepath.Ext(filename)); guessed != "" {
			mimeType = guessed
		} else if mimeType == "" {
			mimeType = "application/octet-stream"
		}
	}

	tmpDir := filepath.Join(s.DataDir, "upload-tmp")
	if strings.TrimSpace(s.DataDir) == "" {
		tmpDir = filepath.Join(os.TempDir(), "nimbus-upload")
	}
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return domain.Node{}, err
	}
	tmp, err := os.CreateTemp(tmpDir, "up-*")
	if err != nil {
		return domain.Node{}, err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, (2<<30)+1))
	if err != nil {
		return domain.Node{}, err
	}
	if written == 0 {
		return domain.Node{}, fmt.Errorf("%w: empty file", domain.ErrValidation)
	}
	if written > 2<<30 {
		return domain.Node{}, fmt.Errorf("%w: file too large", domain.ErrValidation)
	}
	hash := hex.EncodeToString(h.Sum(nil))
	if existing, err := s.Nodes.FindByContentHash(ctx, hash); err == nil {
		existing.Duplicate = true
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Node{}, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return domain.Node{}, err
	}

	channelID, err := s.Blobs.EnsureStorageChannel(ctx)
	if err != nil {
		return domain.Node{}, err
	}

	pid := parentID
	node, err := s.Nodes.Create(ctx, domain.Node{
		ParentID:  &pid,
		Name:      filename,
		Type:      domain.NodeFile,
		MimeType:  mimeType,
		Size:      0,
		Status:    domain.StatusPending,
		ChannelID: channelID,
	})
	if err != nil {
		return domain.Node{}, err
	}

	chunkSize := s.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 8 << 20
	}
	workers := s.UploadWorkers
	if workers <= 0 {
		workers = 3
	}

	buf := make([]byte, chunkSize)
	var (
		mu     sync.Mutex
		parts  []domain.FilePart
		total  int64
		partNo int
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)
	sem := make(chan struct{}, workers)

loop:
	for {
		n, readErr := io.ReadFull(tmp, buf)
		if n > 0 {
			mu.Lock()
			pn := partNo
			partNo++
			total += int64(n)
			mu.Unlock()

			select {
			case <-gctx.Done():
				_ = s.rollbackUpload(ctx, node.ID, snapshotParts(&mu, &parts))
				return domain.Node{}, gctx.Err()
			case sem <- struct{}{}:
			}

			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			g.Go(func() error {
				defer func() { <-sem }()
				msgID, upErr := s.Blobs.UploadPart(gctx, channelID, pn, node.ID, bytes.NewReader(chunk), int64(len(chunk)))
				if upErr != nil {
					return upErr
				}
				mu.Lock()
				parts = append(parts, domain.FilePart{
					ID:        uuid.NewString(),
					FileID:    node.ID,
					PartNo:    pn,
					MessageID: msgID,
					Size:      int64(len(chunk)),
					ChannelID: channelID,
				})
				mu.Unlock()
				return nil
			})
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break loop
		}
		if readErr != nil {
			_ = s.rollbackUpload(ctx, node.ID, snapshotParts(&mu, &parts))
			return domain.Node{}, readErr
		}
	}

	if err := g.Wait(); err != nil {
		_ = s.rollbackUpload(ctx, node.ID, snapshotParts(&mu, &parts))
		return domain.Node{}, err
	}

	mu.Lock()
	finalParts := append([]domain.FilePart(nil), parts...)
	finalTotal := total
	mu.Unlock()
	sort.Slice(finalParts, func(i, j int) bool { return finalParts[i].PartNo < finalParts[j].PartNo })

	if finalTotal == 0 {
		_ = s.rollbackUpload(ctx, node.ID, finalParts)
		return domain.Node{}, fmt.Errorf("%w: empty file", domain.ErrValidation)
	}

	if err := s.Parts.ReplaceParts(ctx, node.ID, finalParts); err != nil {
		_ = s.rollbackUpload(ctx, node.ID, finalParts)
		return domain.Node{}, err
	}
	if err := s.Nodes.UpdateSize(ctx, node.ID, finalTotal); err != nil {
		_ = s.rollbackUpload(ctx, node.ID, finalParts)
		return domain.Node{}, err
	}
	if err := s.Nodes.UpdateStatus(ctx, node.ID, domain.StatusReady); err != nil {
		return domain.Node{}, err
	}
	_ = s.Nodes.SetContentHash(ctx, node.ID, hash)
	node.Size = finalTotal
	node.Status = domain.StatusReady

	// Build image/video thumbs in background so grid cards stay fast.
	if strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(mimeType, "video/") ||
		isImageFilename(filename) || isVideoFilename(filename) {
		go func(id string) {
			_ = s.EnsureThumb(context.Background(), id)
		}(node.ID)
	}
	return node, nil
}

func (s *Services) uniqueChildName(ctx context.Context, parentID, name string) (string, error) {
	if _, err := s.Nodes.FindChildByName(ctx, parentID, name); errors.Is(err, domain.ErrNotFound) {
		return name, nil
	} else if err != nil {
		return "", err
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if _, err := s.Nodes.FindChildByName(ctx, parentID, candidate); errors.Is(err, domain.ErrNotFound) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("%w: name already used", domain.ErrConflict)
}

func snapshotParts(mu *sync.Mutex, parts *[]domain.FilePart) []domain.FilePart {
	mu.Lock()
	defer mu.Unlock()
	out := make([]domain.FilePart, len(*parts))
	copy(out, *parts)
	return out
}

func (s *Services) rollbackUpload(ctx context.Context, fileID string, parts []domain.FilePart) error {
	ids := make([]int, 0, len(parts))
	var channelID int64
	for _, p := range parts {
		ids = append(ids, p.MessageID)
		channelID = p.ChannelID
	}
	_ = s.Blobs.DeleteMessages(ctx, channelID, ids)
	_ = s.Parts.DeleteByFile(ctx, fileID)
	return s.Nodes.SoftDelete(ctx, fileID)
}

func (s *Services) GetNode(ctx context.Context, id string) (domain.Node, error) {
	return s.Nodes.Get(ctx, id)
}

func (s *Services) Download(ctx context.Context, fileID string, w io.Writer) (domain.Node, error) {
	return s.DownloadRange(ctx, fileID, w, 0, -1)
}

// DownloadRange writes bytes [start, endInclusive] to w. endInclusive < 0 means EOF.
func (s *Services) DownloadRange(ctx context.Context, fileID string, w io.Writer, start, endInclusive int64) (domain.Node, error) {
	node, err := s.Nodes.Get(ctx, fileID)
	if err != nil {
		return domain.Node{}, err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		return domain.Node{}, domain.ErrNotFound
	}
	parts, err := s.Parts.ListByFile(ctx, fileID)
	if err != nil {
		return domain.Node{}, err
	}
	if len(parts) == 0 {
		return domain.Node{}, domain.ErrNotFound
	}
	sizes := make([]int64, len(parts))
	var total int64
	for i, p := range parts {
		sizes[i] = p.Size
		total += p.Size
	}
	if endInclusive < 0 || endInclusive >= total {
		endInclusive = total - 1
	}
	if start < 0 || start > endInclusive || start >= total {
		return domain.Node{}, fmt.Errorf("%w: invalid range", domain.ErrValidation)
	}

	// Fast path: serve from local cache (browser seeks no longer re-hit Telegram).
	path := s.mediaCachePath(node.ID)
	if st, err := os.Stat(path); err == nil && st.Size() == total {
		if err := s.serveLocalRange(path, w, start, endInclusive); err != nil {
			return domain.Node{}, err
		}
		return node, nil
	}

	const waitCacheMax = 80 << 20 // ~80MB — wait once so short videos/photos play reliably
	if total > 0 && total <= waitCacheMax {
		cached, err := s.ensureMediaCache(ctx, node, parts)
		if err != nil {
			return domain.Node{}, err
		}
		if err := s.serveLocalRange(cached, w, start, endInclusive); err != nil {
			return domain.Node{}, err
		}
		return node, nil
	}

	// Large files: stream this range from Telegram now; warm disk cache in background.
	go func(n domain.Node, p []domain.FilePart) {
		_, _ = s.ensureMediaCache(context.Background(), n, p)
	}(node, parts)

	ranges, err := domain.MapRangeToParts(sizes, start, endInclusive)
	if err != nil {
		return domain.Node{}, err
	}
	for _, pr := range ranges {
		part := parts[pr.PartNo]
		if err := s.writePartSlice(ctx, part, pr.Skip, pr.Take, w); err != nil {
			return domain.Node{}, err
		}
	}
	return node, nil
}

func (s *Services) writePartSlice(ctx context.Context, part domain.FilePart, skip, take int64, w io.Writer) error {
	if skip == 0 && take == part.Size {
		return s.Blobs.DownloadPart(ctx, part.ChannelID, part.MessageID, w)
	}
	var buf bytes.Buffer
	if part.Size > 0 {
		buf.Grow(int(part.Size))
	}
	if err := s.Blobs.DownloadPart(ctx, part.ChannelID, part.MessageID, &buf); err != nil {
		return err
	}
	data := buf.Bytes()
	if int64(len(data)) < skip+take {
		return fmt.Errorf("short telegram part: have %d want %d+%d", len(data), skip, take)
	}
	_, err := w.Write(data[skip : skip+take])
	return err
}

func (s *Services) Delete(ctx context.Context, id string) error {
	return s.MoveToTrash(ctx, id)
}

func (s *Services) Rename(ctx context.Context, id, name string) (domain.Node, error) {
	name = strings.TrimSpace(name)
	if err := domain.ValidateNodeName(name); err != nil {
		return domain.Node{}, err
	}
	if id == "root" {
		return domain.Node{}, fmt.Errorf("%w: cannot rename root", domain.ErrValidation)
	}
	node, err := s.Nodes.Get(ctx, id)
	if err != nil {
		return domain.Node{}, err
	}
	if node.Name == name {
		return node, nil
	}
	parentID := "root"
	if node.ParentID != nil && *node.ParentID != "" {
		parentID = *node.ParentID
	}
	existing, err := s.Nodes.FindChildByName(ctx, parentID, name)
	if err == nil && existing.ID != id {
		return domain.Node{}, fmt.Errorf("%w: name already used", domain.ErrConflict)
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.Node{}, err
	}
	if err := s.Nodes.Rename(ctx, id, name); err != nil {
		return domain.Node{}, err
	}
	return s.Nodes.Get(ctx, id)
}

func (s *Services) Move(ctx context.Context, id, parentID string) (domain.Node, error) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		parentID = "root"
	}
	if id == "root" {
		return domain.Node{}, fmt.Errorf("%w: cannot move root", domain.ErrValidation)
	}
	if id == parentID {
		return domain.Node{}, fmt.Errorf("%w: cannot move into itself", domain.ErrValidation)
	}
	if _, err := s.Nodes.Get(ctx, id); err != nil {
		return domain.Node{}, err
	}
	if parentID != "root" {
		parent, err := s.Nodes.Get(ctx, parentID)
		if err != nil {
			return domain.Node{}, err
		}
		if parent.Type != domain.NodeFolder {
			return domain.Node{}, fmt.Errorf("%w: destination is not a folder", domain.ErrValidation)
		}
	}
	if err := s.Nodes.Move(ctx, id, parentID); err != nil {
		return domain.Node{}, err
	}
	return s.Nodes.Get(ctx, id)
}

func (s *Services) MoveMany(ctx context.Context, ids []string, parentID string) error {
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		if _, err := s.Move(ctx, id, parentID); err != nil {
			return err
		}
	}
	return nil
}

func isMediaFilename(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".webm", ".mov", ".mkv", ".m4v", ".avi", ".ogv",
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".heic",
		".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac":
		return true
	default:
		return false
	}
}
