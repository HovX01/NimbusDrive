package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vrc/nimbus/internal/domain"
)

type DatabaseSource interface {
	SnapshotToFile(ctx context.Context, path string) error
}

type FileSource interface {
	Download(ctx context.Context, fileID string, w io.Writer) (domain.Node, error)
}

type NodeSource interface {
	Get(ctx context.Context, id string) (domain.Node, error)
}

type JobStatus struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Phase      string    `json:"phase"`
	Progress   int       `json:"progress"`
	Message    string    `json:"message,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type Request struct {
	Database bool     `json:"database"`
	FileIDs  []string `json:"file_ids"`
}

type Manifest struct {
	FormatVersion int            `json:"format_version"`
	JobID         string         `json:"job_id"`
	CreatedAt     string         `json:"created_at"`
	Database      *ManifestItem  `json:"database,omitempty"`
	Files         []ManifestItem `json:"files,omitempty"`
}

type ManifestItem struct {
	Kind   string `json:"kind"`
	NodeID string `json:"node_id,omitempty"`
	Name   string `json:"name,omitempty"`
	Key    string `json:"key"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Status string `json:"status"`
}

type Service struct {
	dataDir  string
	database DatabaseSource
	files    FileSource
	nodes    NodeSource
	mu       sync.RWMutex
	jobs     map[string]*JobStatus
	busy     chan struct{}
}

func NewService(dataDir string, database DatabaseSource, files FileSource, nodes NodeSource) *Service {
	return &Service{
		dataDir:  dataDir,
		database: database,
		files:    files,
		nodes:    nodes,
		jobs:     make(map[string]*JobStatus),
		busy:     make(chan struct{}, 1),
	}
}

func (s *Service) Start(ctx context.Context, req Request) (string, error) {
	if !req.Database && len(req.FileIDs) == 0 {
		return "", fmt.Errorf("%w: database or file_ids required", domain.ErrValidation)
	}
	id := uuid.NewString()
	st := &JobStatus{ID: id, Status: "queued", Phase: "queued"}
	s.mu.Lock()
	s.jobs[id] = st
	s.mu.Unlock()
	go s.run(req, id)
	return id, nil
}

func (s *Service) Status(id string) (JobStatus, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.jobs[id]
	if !ok {
		return JobStatus{}, false
	}
	return *st, true
}

func (s *Service) run(req Request, id string) {
	select {
	case s.busy <- struct{}{}:
	default:
		s.fail(id, fmt.Errorf("another backup is running"))
		return
	}
	defer func() { <-s.busy }()
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	s.update(id, func(st *JobStatus) {
		st.Status = "running"
		st.Phase = "init"
		st.StartedAt = time.Now().UTC()
	})
	cfg, err := LoadConfig(s.dataDir)
	if err != nil || !cfg.Enabled {
		s.fail(id, fmt.Errorf("backup S3 is not configured"))
		return
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretKey, cfg.SessionToken),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
		BucketLookup: func() minio.BucketLookupType {
			if cfg.PathStyle {
				return minio.BucketLookupPath
			}
			return minio.BucketLookupAuto
		}(),
	})
	if err != nil {
		s.fail(id, fmt.Errorf("S3 client: %w", err))
		return
	}
	prefix := cfg.Prefix
	if prefix != "" {
		prefix += "/"
	}
	prefix += time.Now().UTC().Format("20060102T150405Z") + "-" + id
	var manifest Manifest
	manifest.FormatVersion = 1
	manifest.JobID = id
	manifest.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	totalSteps := 0
	if req.Database {
		totalSteps++
	}
	totalSteps += len(req.FileIDs)
	doneSteps := 0
	if req.Database {
		s.update(id, func(st *JobStatus) { st.Phase = "database"; st.Message = "creating snapshot" })
		item, err := s.uploadDatabase(ctx, client, cfg.Bucket, prefix)
		if err != nil {
			s.fail(id, err)
			return
		}
		manifest.Database = item
		doneSteps++
		s.update(id, func(st *JobStatus) { st.Progress = doneSteps * 100 / totalSteps })
	}
	for i, fileID := range req.FileIDs {
		s.update(id, func(st *JobStatus) {
			st.Phase = "files"
			st.Message = fmt.Sprintf("uploading file %d of %d", i+1, len(req.FileIDs))
		})
		item, err := s.uploadFile(ctx, client, cfg.Bucket, prefix, fileID)
		if err != nil {
			s.fail(id, err)
			return
		}
		manifest.Files = append(manifest.Files, *item)
		doneSteps++
		s.update(id, func(st *JobStatus) { st.Progress = doneSteps * 100 / totalSteps })
	}
	s.update(id, func(st *JobStatus) { st.Phase = "manifest"; st.Message = "uploading manifest" })
	if err := s.uploadManifest(ctx, client, cfg.Bucket, prefix, manifest); err != nil {
		s.fail(id, err)
		return
	}
	s.update(id, func(st *JobStatus) {
		st.Status = "done"
		st.Phase = "done"
		st.Progress = 100
		st.FinishedAt = time.Now().UTC()
	})
}

func (s *Service) uploadDatabase(ctx context.Context, client *minio.Client, bucket, prefix string) (*ManifestItem, error) {
	if s.database == nil {
		return nil, fmt.Errorf("database source not configured")
	}
	tmpPath := filepath.Join(s.dataDir, "backup-snapshot-"+uuid.NewString()+".db")
	defer os.Remove(tmpPath)
	if err := s.database.SnapshotToFile(ctx, tmpPath); err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	key := prefix + "/database/nimbus.db"
	hasher := sha256.New()
	reader := io.TeeReader(f, hasher)
	_, err = client.PutObject(ctx, bucket, key, reader, st.Size(), minio.PutObjectOptions{ContentType: "application/vnd.sqlite3"})
	if err != nil {
		return nil, fmt.Errorf("upload database: %w", err)
	}
	return &ManifestItem{
		Kind:   "database",
		Key:    key,
		Size:   st.Size(),
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
		Status: "uploaded",
	}, nil
}

func (s *Service) uploadFile(ctx context.Context, client *minio.Client, bucket, prefix, fileID string) (*ManifestItem, error) {
	node, err := s.nodes.Get(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		return nil, fmt.Errorf("%w: file %s is not ready", domain.ErrValidation, fileID)
	}
	key := prefix + "/files/" + node.ID + "/content"
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		_, dlErr := s.files.Download(ctx, node.ID, pw)
		pw.CloseWithError(dlErr)
		errCh <- dlErr
	}()
	hasher := sha256.New()
	reader := io.TeeReader(pr, hasher)
	_, err = client.PutObject(ctx, bucket, key, reader, node.Size, minio.PutObjectOptions{ContentType: node.MimeType})
	if err != nil {
		return nil, fmt.Errorf("upload file %s: %w", fileID, err)
	}
	if dlErr := <-errCh; dlErr != nil {
		return nil, fmt.Errorf("download file %s: %w", fileID, dlErr)
	}
	return &ManifestItem{
		Kind:   "file",
		NodeID: node.ID,
		Name:   node.Name,
		Key:    key,
		Size:   node.Size,
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
		Status: "uploaded",
	}, nil
}

func (s *Service) uploadManifest(ctx context.Context, client *minio.Client, bucket, prefix string, m Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	reader := bytes.NewReader(b)
	_, err = client.PutObject(ctx, bucket, prefix+"/manifest.json", reader, int64(len(b)), minio.PutObjectOptions{ContentType: "application/json"})
	return err
}

func (s *Service) update(id string, fn func(*JobStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.jobs[id]; st != nil {
		fn(st)
	}
}

func (s *Service) fail(id string, err error) {
	s.update(id, func(st *JobStatus) {
		st.Status = "error"
		st.Phase = "error"
		st.Error = err.Error()
		st.FinishedAt = time.Now().UTC()
	})
}
