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
	ID        string        `json:"id"`
	Status    string        `json:"status"` // queued|running|done|error
	Phase     string        `json:"phase"`  // download|upload|done|error
	Progress  float64       `json:"progress"`
	Message   string        `json:"message"`
	ErrorCode string        `json:"error_code,omitempty"`
	Service   string        `json:"service,omitempty"`
	Node      *domain.Node  `json:"node,omitempty"`  // first file (compat)
	Nodes     []domain.Node `json:"nodes,omitempty"` // all files from multi-media posts
	Count     int           `json:"count,omitempty"`
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
	nodes     []domain.Node
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
		Count:     len(j.nodes),
	}
	if j.node != nil {
		n := *j.node
		out.Node = &n
	}
	if len(j.nodes) > 0 {
		out.Nodes = append([]domain.Node(nil), j.nodes...)
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
		AuthToken: s.socialBearerForService(ctx, job.service),
		OnProgress: func(pct float64, msg string) {
			if pct >= 0 {
				job.set("running", "download", msg, pct*0.85)
			} else {
				job.set("running", "download", msg, -1)
			}
		},
	}
	if cookies := s.socialCookiesForService(ctx, job.service); cookies != "" {
		opt.Cookies = cookies
	}

	var paths []string
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
			paths = p
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
	if len(paths) == 0 {
		if lastClass.Code == "" {
			lastClass = fetch.Classified{Code: fetch.CodeFetchFail, Message: "Download failed"}
		}
		job.fail(lastClass.Code, lastClass.Message)
		return
	}

	jobDir := filepath.Dir(paths[0])
	defer os.RemoveAll(jobDir)

	nodes := make([]domain.Node, 0, len(paths))
	dupes := 0
	for i, path := range paths {
		job.set("running", "upload", fmt.Sprintf("Uploading %d/%d to Drive…", i+1, len(paths)), 88+float64(i)*10/float64(len(paths)))
		f, err := os.Open(path)
		if err != nil {
			job.fail(fetch.CodeFetchFail, err.Error())
			return
		}
		info, err := f.Stat()
		if err != nil {
			_ = f.Close()
			job.fail(fetch.CodeFetchFail, err.Error())
			return
		}
		name := domain.SanitizeDownloadName(filepath.Base(path))
		node, err := s.Upload(ctx, parentID, name, "", f, info.Size())
		_ = f.Close()
		if err != nil {
			job.fail(fetch.CodeFetchFail, err.Error())
			return
		}
		if node.Duplicate {
			dupes++
		}
		nodes = append(nodes, node)
	}

	job.mu.Lock()
	job.nodes = nodes
	if len(nodes) > 0 {
		n := nodes[0]
		job.node = &n
	}
	job.status = "done"
	job.phase = "done"
	job.progress = 100
	switch {
	case len(nodes) == 1 && nodes[0].Duplicate:
		job.message = "Already in Drive (duplicate skipped)"
	case len(nodes) == 1:
		job.message = "Saved to Drive"
	case dupes == len(nodes):
		job.message = fmt.Sprintf("All %d files already in Drive", len(nodes))
	case dupes > 0:
		job.message = fmt.Sprintf("Saved %d files (%d already in Drive)", len(nodes)-dupes, dupes)
	default:
		job.message = fmt.Sprintf("Saved %d files to Drive", len(nodes))
	}
	job.errorCode = ""
	job.mu.Unlock()
}

func (s *Services) downloadNative(ctx context.Context, job *fetchJob, rawURL string, opt fetch.Options) ([]string, error) {
	job.set("running", "download", "Extracting media…", 5)
	tmpRoot := filepath.Join(s.DataDir, "fetch-tmp")
	jobDir := filepath.Join(tmpRoot, job.id+"-"+uuid.NewString()[:8])
	if err := os.MkdirAll(jobDir, 0o700); err != nil {
		return nil, err
	}
	opt.TempDir = jobDir
	media, err := fetch.Resolve(ctx, rawURL, opt)
	if err != nil {
		_ = os.RemoveAll(jobDir)
		return nil, err
	}
	if media.Service != "" {
		job.mu.Lock()
		job.service = media.Service
		job.mu.Unlock()
	}

	n := len(media.AllItems())
	if n > 1 {
		job.set("running", "download", fmt.Sprintf("Downloading %d files…", n), 8)
	} else {
		job.set("running", "download", "Downloading…", 8)
	}
	paths, err := fetch.DownloadAllToFiles(ctx, media, jobDir, opt.OnProgress)
	if err != nil {
		_ = os.RemoveAll(jobDir)
		return nil, err
	}
	job.set("running", "download", "Download finished", 85)
	return paths, nil
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
