package app

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
	"github.com/vrc/nimbus/internal/fetch"
)

// FetchJobStatus is one async URL→Drive job (Cobalt-style extract + progress).
type FetchJobStatus struct {
	ID        string       `json:"id"`
	Status    string       `json:"status"` // queued|running|done|error
	Phase     string       `json:"phase"`  // download|upload|done|error
	Progress  float64      `json:"progress"`
	Message   string       `json:"message"`
	ErrorCode string       `json:"error_code,omitempty"`
	Service   string       `json:"service,omitempty"`
	Node      *domain.Node `json:"node,omitempty"`
}

type fetchJob struct {
	mu        sync.Mutex
	id        string
	status    string
	phase     string
	progress  float64
	message   string
	errorCode string
	service   string
	node      *domain.Node
}

func (j *fetchJob) snapshot() FetchJobStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := FetchJobStatus{
		ID:        j.id,
		Status:    j.status,
		Phase:     j.phase,
		Progress:  j.progress,
		Message:   j.message,
		ErrorCode: j.errorCode,
		Service:   j.service,
	}
	if j.node != nil {
		n := *j.node
		out.Node = &n
	}
	return out
}

func (j *fetchJob) set(status, phase, message string, progress float64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if status != "" {
		j.status = status
	}
	if phase != "" {
		j.phase = phase
	}
	if message != "" {
		j.message = message
	}
	if progress >= 0 {
		if progress > 100 {
			progress = 100
		}
		if progress > j.progress {
			j.progress = progress
		}
	}
}

func (j *fetchJob) fail(code fetch.ErrorCode, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = "error"
	j.phase = "error"
	j.errorCode = string(code)
	j.message = message
}

type fetchHub struct {
	mu   sync.Mutex
	jobs map[string]*fetchJob
}

func (s *Services) hub() *fetchHub {
	s.fetchOnce.Do(func() {
		s.fetchJobs = &fetchHub{jobs: make(map[string]*fetchJob)}
	})
	return s.fetchJobs
}

// StartFetchURL begins an async Cobalt-style extract+download that lands in Drive.
func (s *Services) StartFetchURL(parentID, rawURL, mode string, maxHeight int) (string, error) {
	normalized, err := fetch.NormalizeURL(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: %s", domain.ErrValidation, err.Error())
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "video"
	}
	if mode != "video" && mode != "audio" {
		return "", fmt.Errorf("%w: mode must be video or audio", domain.ErrValidation)
	}
	if maxHeight < 0 {
		return "", fmt.Errorf("%w: max_height invalid", domain.ErrValidation)
	}
	maxHeight = fetch.SnapHeight(maxHeight)

	id := uuid.NewString()
	job := &fetchJob{
		id:      id,
		status:  "queued",
		phase:   "download",
		message: "Queued…",
		service: fetch.DetectService(normalized),
	}
	h := s.hub()
	h.mu.Lock()
	h.jobs[id] = job
	h.mu.Unlock()

	go s.runFetchJob(id, parentID, normalized, mode, maxHeight)
	return id, nil
}

// GetFetchJob returns current progress for an async fetch.
func (s *Services) GetFetchJob(id string) (FetchJobStatus, error) {
	h := s.hub()
	h.mu.Lock()
	job := h.jobs[id]
	h.mu.Unlock()
	if job == nil {
		return FetchJobStatus{}, domain.ErrNotFound
	}
	return job.snapshot(), nil
}

func (s *Services) runFetchJob(jobID, parentID, rawURL, mode string, maxHeight int) {
	h := s.hub()
	h.mu.Lock()
	job := h.jobs[jobID]
	h.mu.Unlock()
	if job == nil {
		return
	}

	job.set("running", "download", fmt.Sprintf("Resolving %s…", job.service), 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	seed := hashSeed(rawURL + jobID)
	maxAttempts := fetch.MaxAttemptsFor(job.service)
	opt := fetch.Options{
		Mode:      mode,
		MaxHeight: maxHeight,
		Cookies:   s.cookiesPath(),
		OnProgress: func(pct float64, msg string) {
			if pct >= 0 {
				job.set("running", "download", msg, pct*0.85)
			} else {
				job.set("running", "download", msg, -1)
			}
		},
	}

	var path string
	var lastClass fetch.Classified

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(fetch.BackoffMs(attempt-1, seed)) * time.Millisecond
			job.set("running", "download", fmt.Sprintf("Retry %d/%d…", attempt+1, maxAttempts), -1)
			select {
			case <-ctx.Done():
				job.fail(fetch.CodeFetchFail, "Cancelled")
				return
			case <-time.After(delay):
			}
		}

		p, err := s.downloadNative(ctx, job, rawURL, opt)
		if err == nil {
			path = p
			lastClass = fetch.Classified{}
			break
		}
		lastClass = fetch.AsClassified(err)
		if lastClass.Code == "" {
			lastClass = fetch.Classify(err.Error(), job.service)
		}
		if !fetch.ShouldRetryAttempt(attempt, maxAttempts, lastClass) {
			job.fail(lastClass.Code, lastClass.Message)
			return
		}
		job.set("running", "download", lastClass.Message, -1)
	}
	if path == "" {
		if lastClass.Code == "" {
			lastClass = fetch.Classified{Code: fetch.CodeFetchFail, Message: "Download failed"}
		}
		job.fail(lastClass.Code, lastClass.Message)
		return
	}

	job.set("running", "upload", "Uploading to Drive…", 88)
	f, err := os.Open(path)
	if err != nil {
		job.fail(fetch.CodeFetchFail, err.Error())
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		job.fail(fetch.CodeFetchFail, err.Error())
		return
	}
	name := domain.SanitizeDownloadName(filepath.Base(path))
	node, err := s.Upload(ctx, parentID, name, "", f, info.Size())
	_ = os.RemoveAll(filepath.Dir(path))
	if err != nil {
		job.fail(fetch.CodeFetchFail, err.Error())
		return
	}
	job.mu.Lock()
	job.node = &node
	job.status = "done"
	job.phase = "done"
	job.progress = 100
	job.message = "Saved to Drive"
	job.errorCode = ""
	job.mu.Unlock()
}

func (s *Services) downloadNative(ctx context.Context, job *fetchJob, rawURL string, opt fetch.Options) (string, error) {
	job.set("running", "download", "Extracting media URL…", 5)
	media, err := fetch.Resolve(ctx, rawURL, opt)
	if err != nil {
		return "", err
	}
	if media.Service != "" {
		job.mu.Lock()
		job.service = media.Service
		job.mu.Unlock()
	}

	tmpRoot := filepath.Join(s.DataDir, "fetch-tmp")
	jobDir := filepath.Join(tmpRoot, job.id+"-"+uuid.NewString()[:8])
	if err := os.MkdirAll(jobDir, 0o700); err != nil {
		return "", err
	}

	job.set("running", "download", "Downloading…", 8)
	path, err := fetch.DownloadToFile(ctx, media, jobDir, opt.OnProgress)
	if err != nil {
		_ = os.RemoveAll(jobDir)
		return "", err
	}
	job.set("running", "download", "Download finished", 85)
	return path, nil
}

func (s *Services) cookiesPath() string {
	if s == nil {
		return ""
	}
	candidates := []string{
		strings.TrimSpace(os.Getenv("NIMBUS_COOKIES")),
		filepath.Join(s.DataDir, "cookies.txt"),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		st, err := os.Stat(p)
		if err == nil && st.Size() > 0 {
			return p
		}
	}
	return ""
}

func hashSeed(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}
