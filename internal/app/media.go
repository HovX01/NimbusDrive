package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
)

// Media presets hybridize PixelPulseBot (Telegram FFmpeg bot) + ffmpeg-web (web media UI).
const (
	MediaActionEnhance  = "enhance"  // 1080p + denoise/sharpen/color (PixelPulse-style)
	MediaActionCompress = "compress" // x265 smaller file (PixelPulse-style)
	MediaActionPlayable = "playable" // H.264/AAC MP4 for browser playback (ffmpeg-web convert spirit)
	MediaActionExtract  = "extract_audio"
)

const (
	mediaEnhanceMaxBytes  = 500 * 1024 * 1024
	mediaCompressMaxBytes = 2 * 1024 * 1024 * 1024
	mediaPlayableMaxBytes = 1024 * 1024 * 1024
)

var mediaTimeRe = regexp.MustCompile(`time=(\d+):(\d+):(\d+(?:\.\d+)?)`)

// MediaJobStatus is one async FFmpeg job.
type MediaJobStatus struct {
	ID       string       `json:"id"`
	Action   string       `json:"action"`
	Status   string       `json:"status"` // queued|running|done|error
	Phase    string       `json:"phase"`  // download|encode|upload|done|error
	Progress float64      `json:"progress"`
	Message  string       `json:"message"`
	Node     *domain.Node `json:"node,omitempty"`
}

type mediaJob struct {
	mu       sync.Mutex
	id       string
	action   string
	status   string
	phase    string
	progress float64
	message  string
	node     *domain.Node
}

func (j *mediaJob) snapshot() MediaJobStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := MediaJobStatus{
		ID:       j.id,
		Action:   j.action,
		Status:   j.status,
		Phase:    j.phase,
		Progress: j.progress,
		Message:  j.message,
	}
	if j.node != nil {
		n := *j.node
		out.Node = &n
	}
	return out
}

func (j *mediaJob) set(status, phase, message string, progress float64) {
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

func (j *mediaJob) fail(message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = "error"
	j.phase = "error"
	j.message = message
}

type mediaHub struct {
	mu   sync.Mutex
	jobs map[string]*mediaJob
}

func (s *Services) mediaHub() *mediaHub {
	s.mediaOnce.Do(func() {
		s.mediaJobs = &mediaHub{jobs: make(map[string]*mediaJob)}
	})
	return s.mediaJobs
}

// StartMediaJob runs FFmpeg on a drive file and uploads the result beside it.
func (s *Services) StartMediaJob(ctx context.Context, fileID, action string) (string, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case MediaActionEnhance, MediaActionCompress, MediaActionPlayable, MediaActionExtract:
	default:
		return "", fmt.Errorf("%w: action must be enhance, compress, playable, or extract_audio", domain.ErrValidation)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("%w: ffmpeg not installed on server", domain.ErrNotConfigured)
	}

	node, err := s.Nodes.Get(ctx, fileID)
	if err != nil {
		return "", err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady {
		return "", fmt.Errorf("%w: only ready files can be processed", domain.ErrValidation)
	}
	if action == MediaActionExtract {
		if !isVideoFile(node.Name, node.MimeType) {
			return "", fmt.Errorf("%w: extract_audio requires a video file", domain.ErrValidation)
		}
	} else if !isMediaFile(node.Name, node.MimeType) {
		return "", fmt.Errorf("%w: not a video/audio file", domain.ErrValidation)
	}
	limit := mediaPlayableMaxBytes
	switch action {
	case MediaActionEnhance:
		limit = mediaEnhanceMaxBytes
	case MediaActionCompress:
		limit = mediaCompressMaxBytes
	}
	if node.Size > int64(limit) {
		return "", fmt.Errorf("%w: file exceeds %s limit for %s", domain.ErrValidation, humanBytes(int64(limit)), action)
	}

	jobID := uuid.NewString()
	job := &mediaJob{
		id:      jobID,
		action:  action,
		status:  "queued",
		phase:   "queued",
		message: "Queued…",
	}
	hub := s.mediaHub()
	hub.mu.Lock()
	hub.jobs[jobID] = job
	hub.mu.Unlock()

	go s.runMediaJob(job, node)
	return jobID, nil
}

func (s *Services) MediaJobStatus(jobID string) (MediaJobStatus, error) {
	hub := s.mediaHub()
	hub.mu.Lock()
	job := hub.jobs[jobID]
	hub.mu.Unlock()
	if job == nil {
		return MediaJobStatus{}, domain.ErrNotFound
	}
	return job.snapshot(), nil
}

func (s *Services) runMediaJob(job *mediaJob, node domain.Node) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	tmpDir, err := os.MkdirTemp(s.DataDir, "media-*")
	if err != nil {
		job.fail(err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "input"+extOf(node.Name))
	outName, outPath, args := mediaArgs(job.action, node.Name, inPath, tmpDir)

	job.set("running", "download", "Downloading from Telegram…", 2)
	f, err := os.Create(inPath)
	if err != nil {
		job.fail(err.Error())
		return
	}
	_, err = s.Download(ctx, node.ID, f)
	_ = f.Close()
	if err != nil {
		job.fail("download failed: " + err.Error())
		return
	}

	duration := probeDuration(ctx, inPath)
	job.set("running", "encode", "Encoding with FFmpeg…", 8)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		job.fail(err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		job.fail("ffmpeg start: " + err.Error())
		return
	}
	go trackFFmpegProgress(stderr, duration, func(pct float64, msg string) {
		// Map encode to 8–85%
		mapped := 8 + pct*0.77
		job.set("running", "encode", msg, mapped)
	})
	if err := cmd.Wait(); err != nil {
		job.fail("ffmpeg failed: " + err.Error())
		return
	}
	st, err := os.Stat(outPath)
	if err != nil || st.Size() == 0 {
		job.fail("ffmpeg produced empty output")
		return
	}

	job.set("running", "upload", "Uploading result to Nimbus…", 88)
	parent := "root"
	if node.ParentID != nil && *node.ParentID != "" {
		parent = *node.ParentID
	}
	rf, err := os.Open(outPath)
	if err != nil {
		job.fail(err.Error())
		return
	}
	defer rf.Close()

	mime := "video/mp4"
	if strings.HasSuffix(strings.ToLower(outName), ".m4a") {
		mime = "audio/mp4"
	}
	if strings.HasSuffix(strings.ToLower(outName), ".mkv") {
		mime = "video/x-matroska"
	}
	outNode, err := s.Upload(ctx, parent, outName, mime, rf, st.Size())
	if err != nil {
		job.fail("upload failed: " + err.Error())
		return
	}

	job.mu.Lock()
	job.node = &outNode
	job.status = "done"
	job.phase = "done"
	job.progress = 100
	job.message = "Done — saved as " + outNode.Name
	job.mu.Unlock()
}

func mediaArgs(action, originalName, inPath, tmpDir string) (outName, outPath string, args []string) {
	base := strings.TrimSuffix(filepath.Base(originalName), filepath.Ext(originalName))
	if base == "" {
		base = "media"
	}
	switch action {
	case MediaActionExtract:
		outName = base + "_audio.m4a"
		outPath = filepath.Join(tmpDir, outName)
		args = []string{
			"-y", "-i", inPath,
			"-vn",
			"-c:a", "aac", "-b:a", "192k",
			"-movflags", "+faststart",
			outPath,
		}
	case MediaActionEnhance:
		// PixelPulseBot filters + better quality than their ultrafast/crf28
		outName = base + "_enhanced.mp4"
		outPath = filepath.Join(tmpDir, outName)
		args = []string{
			"-y", "-i", inPath,
			"-vf", "scale=1920:1080:flags=lanczos:force_original_aspect_ratio=decrease,hqdn3d,unsharp=5:5:1.0:5:5:0.0,eq=contrast=1.15:brightness=0.03:saturation=1.15",
			"-c:v", "libx264", "-preset", "medium", "-crf", "20",
			"-c:a", "aac", "-b:a", "192k",
			"-movflags", "+faststart",
			outPath,
		}
	case MediaActionCompress:
		outName = base + "_compressed.mp4"
		outPath = filepath.Join(tmpDir, outName)
		args = []string{
			"-y", "-i", inPath,
			"-c:v", "libx265", "-preset", "medium", "-crf", "24", "-tag:v", "hvc1",
			"-c:a", "aac", "-b:a", "128k",
			"-movflags", "+faststart",
			outPath,
		}
	default: // playable
		outName = base + "_playable.mp4"
		outPath = filepath.Join(tmpDir, outName)
		args = []string{
			"-y", "-i", inPath,
			"-c:v", "libx264", "-preset", "medium", "-crf", "22",
			"-c:a", "aac", "-b:a", "160k",
			"-movflags", "+faststart",
			"-pix_fmt", "yuv420p",
			outPath,
		}
	}
	return outName, outPath, args
}

func probeDuration(ctx context.Context, path string) float64 {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return f
}

func trackFFmpegProgress(r io.Reader, duration float64, onProgress func(pct float64, msg string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		m := mediaTimeRe.FindStringSubmatch(line)
		if m == nil || duration <= 0 {
			continue
		}
		h, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		sec, _ := strconv.ParseFloat(m[3], 64)
		cur := float64(h*3600+min*60) + sec
		pct := (cur / duration) * 100
		if pct > 100 {
			pct = 100
		}
		onProgress(pct, fmt.Sprintf("Encoding… %.0f%%", pct))
	}
}

func isMediaFile(name, mime string) bool {
	mime = strings.ToLower(mime)
	if strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "audio/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".mp3", ".m4a", ".wav", ".aac", ".flac", ".ogg":
		return true
	default:
		return false
	}
}

func extOf(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return ".bin"
	}
	return ext
}

func humanBytes(n int64) string {
	const kb = 1024
	switch {
	case n >= kb*kb*kb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(kb*kb*kb))
	case n >= kb*kb:
		return fmt.Sprintf("%.0f MB", float64(n)/float64(kb*kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
