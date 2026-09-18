package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
)

type TimelineData struct {
	Duration float64         `json:"duration"`
	Tracks   []TimelineTrack `json:"tracks"`
}

type TimelineTrack struct {
	ID    string         `json:"id"`
	Type  string         `json:"type"`
	Clips []TimelineClip `json:"clips"`
	Muted bool           `json:"muted,omitempty"`
}

type TimelineClip struct {
	ID              string       `json:"id"`
	SourceNodeID    string       `json:"sourceNodeId"`
	StartInSource   float64      `json:"startInSource"`
	EndInSource     float64      `json:"endInSource"`
	StartOnTimeline float64      `json:"startOnTimeline"`
	Speed           float64      `json:"speed,omitempty"`
	HasEffects      bool         `json:"hasEffects,omitempty"`
	Volume          float64      `json:"volume,omitempty"`
	FadeIn          float64      `json:"fade_in,omitempty"`
	FadeOut         float64      `json:"fade_out,omitempty"`
	Text            *TextOverlay `json:"text,omitempty"`
	Transition      *Transition  `json:"transition,omitempty"`
}

type TextOverlay struct {
	Content    string  `json:"content"`
	FontSize   int     `json:"font_size"`
	FontFamily string  `json:"font_family"`
	Color      string  `json:"color"`
	BgColor    string  `json:"bg_color"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Alignment  string  `json:"alignment"`
	Animation  string  `json:"animation"`
}

type Transition struct {
	Type     string  `json:"type"`
	Duration float64 `json:"duration"`
}

type FileProbe struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	VideoCodec string  `json:"video_codec"`
	AudioCodec string  `json:"audio_codec"`
	HasAudio   bool    `json:"has_audio"`
}

type ProxyStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type proxyJob struct {
	mu      sync.Mutex
	status  string
	message string
	path    string
}

type proxyHub struct {
	mu   sync.Mutex
	jobs map[string]*proxyJob
}

type editorSource struct {
	Node  domain.Node
	Path  string
	Probe FileProbe
	Index int
}

type EditExportJobStatus struct {
	ID        string       `json:"id"`
	ProjectID string       `json:"project_id"`
	Status    string       `json:"status"`
	Phase     string       `json:"phase"`
	Progress  float64      `json:"progress"`
	Message   string       `json:"message"`
	Node      *domain.Node `json:"node,omitempty"`
}

type editExportJob struct {
	mu        sync.Mutex
	id        string
	projectID string
	status    string
	phase     string
	progress  float64
	message   string
	node      *domain.Node
	finished  time.Time
}

type editExportHub struct {
	mu   sync.Mutex
	jobs map[string]*editExportJob
}

func (s *Services) editHub() *editExportHub {
	s.editExportOnce.Do(func() {
		s.editExportJobs = &editExportHub{jobs: make(map[string]*editExportJob)}
	})
	return s.editExportJobs
}

func (s *Services) proxyHub() *proxyHub {
	s.proxyOnce.Do(func() {
		s.proxyJobs = &proxyHub{jobs: make(map[string]*proxyJob)}
	})
	return s.proxyJobs
}

func (j *editExportJob) snapshot() EditExportJobStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := EditExportJobStatus{
		ID:        j.id,
		ProjectID: j.projectID,
		Status:    j.status,
		Phase:     j.phase,
		Progress:  j.progress,
		Message:   j.message,
	}
	if j.node != nil {
		n := *j.node
		out.Node = &n
	}
	return out
}

func (j *editExportJob) set(status, phase, message string, progress float64) {
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

func (j *editExportJob) fail(message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = "error"
	j.phase = "error"
	j.message = message
	j.finished = time.Now()
}

func (s *Services) CreateEditProject(ctx context.Context, sourceNodeID string) (domain.EditProject, error) {
	if s.Edits == nil {
		return domain.EditProject{}, domain.ErrNotConfigured
	}
	node, err := s.Nodes.Get(ctx, sourceNodeID)
	if err != nil {
		return domain.EditProject{}, err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady || !isVisualFile(node.Name, node.MimeType) {
		return domain.EditProject{}, fmt.Errorf("%w: source must be a ready video or image file", domain.ErrValidation)
	}
	if existing, err := s.Edits.GetProjectBySource(ctx, sourceNodeID); err == nil {
		return existing, nil
	} else if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.EditProject{}, err
	}
	initialDuration := 0.0
	if isImageFile(node.Name, node.MimeType) {
		initialDuration = 5
	}
	timeline := TimelineData{
		Duration: initialDuration,
		Tracks: []TimelineTrack{{
			ID:   "v1",
			Type: "video",
			Clips: []TimelineClip{{
				ID:              uuid.NewString(),
				SourceNodeID:    node.ID,
				StartInSource:   0,
				EndInSource:     initialDuration,
				StartOnTimeline: 0,
			}},
		}},
	}
	b, err := json.Marshal(timeline)
	if err != nil {
		return domain.EditProject{}, err
	}
	name := strings.TrimSuffix(filepath.Base(node.Name), filepath.Ext(node.Name))
	if name == "" {
		name = node.Name
	}
	return s.Edits.CreateProject(ctx, domain.EditProject{
		SourceNodeID: node.ID,
		Name:         name + " edit project",
		TimelineJSON: string(b),
	})
}

func (s *Services) GetEditProject(ctx context.Context, projectID string) (domain.EditProject, error) {
	if s.Edits == nil {
		return domain.EditProject{}, domain.ErrNotConfigured
	}
	return s.Edits.GetProject(ctx, projectID)
}

func (s *Services) ListEditProjects(ctx context.Context) ([]domain.EditProject, error) {
	if s.Edits == nil {
		return nil, domain.ErrNotConfigured
	}
	return s.Edits.ListProjects(ctx)
}

func (s *Services) SaveEditTimeline(ctx context.Context, projectID, timelineJSON string) error {
	if s.Edits == nil {
		return domain.ErrNotConfigured
	}
	if strings.TrimSpace(timelineJSON) == "" {
		return fmt.Errorf("%w: timeline_json required", domain.ErrValidation)
	}
	var timeline TimelineData
	if err := json.Unmarshal([]byte(timelineJSON), &timeline); err != nil {
		return fmt.Errorf("%w: invalid timeline_json", domain.ErrValidation)
	}
	return s.Edits.UpdateTimeline(ctx, projectID, timelineJSON)
}

func (s *Services) DeleteEditProject(ctx context.Context, projectID string) error {
	if s.Edits == nil {
		return domain.ErrNotConfigured
	}
	return s.Edits.DeleteProject(ctx, projectID)
}

func (s *Services) StartEditExport(ctx context.Context, projectID, preset string) (string, error) {
	if s.Edits == nil {
		return "", domain.ErrNotConfigured
	}
	preset = strings.ToLower(strings.TrimSpace(preset))
	if preset == "" {
		preset = "match"
	}
	switch preset {
	case "match", "1080p", "compressed":
	default:
		return "", fmt.Errorf("%w: preset must be match, 1080p, or compressed", domain.ErrValidation)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("%w: ffmpeg not installed on server", domain.ErrNotConfigured)
	}
	s.purgeOldEditExportJobs()
	project, err := s.Edits.GetProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	node, err := s.Nodes.Get(ctx, project.SourceNodeID)
	if err != nil {
		return "", err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady || !isVisualFile(node.Name, node.MimeType) {
		return "", fmt.Errorf("%w: source must be a ready video or image file", domain.ErrValidation)
	}

	jobID := uuid.NewString()
	job := &editExportJob{
		id:        jobID,
		projectID: project.ID,
		status:    "queued",
		phase:     "queued",
		message:   "Queued...",
	}
	hub := s.editHub()
	hub.mu.Lock()
	hub.jobs[jobID] = job
	hub.mu.Unlock()

	go s.runEditExport(job, project, node, preset)
	return jobID, nil
}

func (s *Services) GetEditExportStatus(jobID string) (EditExportJobStatus, error) {
	hub := s.editHub()
	hub.mu.Lock()
	job := hub.jobs[jobID]
	hub.mu.Unlock()
	if job == nil {
		return EditExportJobStatus{}, domain.ErrNotFound
	}
	return job.snapshot(), nil
}

func (s *Services) runEditExport(job *editExportJob, project domain.EditProject, source domain.Node, preset string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()

	tmpDir, err := os.MkdirTemp(s.DataDir, "edit-*")
	if err != nil {
		job.fail(err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	timeline, err := parseTimeline(project.TimelineJSON, source.ID, 0)
	if err != nil {
		job.fail(err.Error())
		return
	}
	job.set("running", "download", "Downloading project media from Telegram...", 2)
	sources, err := s.downloadEditorSources(ctx, tmpDir, timeline, source)
	if err != nil {
		job.fail(err.Error())
		return
	}
	timeline, err = parseTimeline(project.TimelineJSON, source.ID, sources[source.ID].Probe.Duration)
	if err != nil {
		job.fail(err.Error())
		return
	}
	outName, outPath, args := editExportArgs(timeline, source.Name, sources, tmpDir, preset)

	job.set("running", "render", "Rendering with FFmpeg...", 8)
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
	go trackFFmpegProgress(stderr, timeline.Duration, func(pct float64, msg string) {
		job.set("running", "render", msg, 8+pct*0.78)
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

	parent := "root"
	if source.ParentID != nil && *source.ParentID != "" {
		parent = *source.ParentID
	}
	job.set("running", "upload", "Uploading export to Nimbus...", 88)
	rf, err := os.Open(outPath)
	if err != nil {
		job.fail(err.Error())
		return
	}
	defer rf.Close()
	outNode, err := s.Upload(ctx, parent, outName, "video/mp4", rf, st.Size())
	if err != nil {
		job.fail("upload failed: " + err.Error())
		return
	}
	job.mu.Lock()
	job.node = &outNode
	job.status = "done"
	job.phase = "done"
	job.progress = 100
	job.message = "Done - saved as " + outNode.Name
	job.finished = time.Now()
	job.mu.Unlock()
}

func (s *Services) purgeOldEditExportJobs() {
	hub := s.editHub()
	cutoff := time.Now().Add(-30 * time.Minute)
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for id, job := range hub.jobs {
		job.mu.Lock()
		done := (job.status == "done" || job.status == "error") && !job.finished.IsZero() && job.finished.Before(cutoff)
		job.mu.Unlock()
		if done {
			delete(hub.jobs, id)
		}
	}
}

func parseTimeline(raw, fallbackSourceID string, fallbackDuration float64) (TimelineData, error) {
	var timeline TimelineData
	if err := json.Unmarshal([]byte(raw), &timeline); err != nil {
		return TimelineData{}, fmt.Errorf("%w: invalid timeline", domain.ErrValidation)
	}
	var tracks []TimelineTrack
	var renderClips []TimelineClip
	for _, track := range timeline.Tracks {
		if track.ID == "" {
			track.ID = "track-" + uuid.NewString()
		}
		for i := range track.Clips {
			if track.Clips[i].ID == "" {
				track.Clips[i].ID = uuid.NewString()
			}
			if track.Clips[i].SourceNodeID == "" {
				track.Clips[i].SourceNodeID = fallbackSourceID
			}
			if track.Clips[i].Speed <= 0 {
				track.Clips[i].Speed = 1
			}
			track.Clips[i].Speed = normalizedSpeed(track.Clips[i].Speed)
			if track.Clips[i].Volume <= 0 {
				track.Clips[i].Volume = 1
			}
			if track.Type == "text" || track.Type == "caption" {
				if track.Clips[i].EndInSource <= track.Clips[i].StartInSource {
					track.Clips[i].EndInSource = track.Clips[i].StartInSource + 3
				}
				continue
			}
			if track.Clips[i].EndInSource <= 0 {
				if fallbackDuration > 0 {
					track.Clips[i].EndInSource = fallbackDuration
				} else {
					track.Clips[i].EndInSource = track.Clips[i].StartInSource + 0.1
				}
			}
			if track.Clips[i].EndInSource <= track.Clips[i].StartInSource {
				return TimelineData{}, fmt.Errorf("%w: clip end must be after start", domain.ErrValidation)
			}
		}
		if track.Type == "video" {
			renderClips = append(renderClips, track.Clips...)
		}
		if track.Type != "" {
			tracks = append(tracks, track)
		}
	}
	if len(renderClips) == 0 {
		return TimelineData{}, fmt.Errorf("%w: timeline has no video clips", domain.ErrValidation)
	}
	sort.Slice(renderClips, func(i, j int) bool { return renderClips[i].StartOnTimeline < renderClips[j].StartOnTimeline })
	total := timeline.Duration
	for _, track := range tracks {
		for _, clip := range track.Clips {
			d := clipDuration(clip)
			if track.Type == "text" || track.Type == "caption" {
				d = clip.EndInSource - clip.StartInSource
			}
			if end := clip.StartOnTimeline + d; end > total {
				total = end
			}
		}
	}
	timeline.Tracks = tracks
	timeline.Duration = total
	return timeline, nil
}

func videoClips(timeline TimelineData) []TimelineClip {
	var clips []TimelineClip
	for _, track := range timeline.Tracks {
		if track.Type != "video" || track.Muted {
			continue
		}
		clips = append(clips, track.Clips...)
	}
	sort.Slice(clips, func(i, j int) bool { return clips[i].StartOnTimeline < clips[j].StartOnTimeline })
	return clips
}

func audioClips(timeline TimelineData) []TimelineClip {
	var clips []TimelineClip
	for _, track := range timeline.Tracks {
		if track.Type != "audio" || track.Muted {
			continue
		}
		clips = append(clips, track.Clips...)
	}
	sort.Slice(clips, func(i, j int) bool { return clips[i].StartOnTimeline < clips[j].StartOnTimeline })
	return clips
}

func (s *Services) downloadEditorSources(ctx context.Context, tmpDir string, timeline TimelineData, primary domain.Node) (map[string]editorSource, error) {
	ids := map[string]bool{primary.ID: true}
	for _, track := range timeline.Tracks {
		for _, clip := range track.Clips {
			if strings.TrimSpace(clip.SourceNodeID) != "" {
				ids[clip.SourceNodeID] = true
			}
		}
	}
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	sources := make(map[string]editorSource, len(keys))
	for index, id := range keys {
		node := primary
		if id != primary.ID {
			var err error
			node, err = s.Nodes.Get(ctx, id)
			if err != nil {
				return nil, err
			}
		}
		if node.Type != domain.NodeFile || node.Status != domain.StatusReady || !isMediaFile(node.Name, node.MimeType) && !isImageFile(node.Name, node.MimeType) {
			return nil, fmt.Errorf("%w: timeline source must be ready media", domain.ErrValidation)
		}
		path := filepath.Join(tmpDir, fmt.Sprintf("source-%d%s", index, extOf(node.Name)))
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		_, err = s.Download(ctx, node.ID, f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("download failed for %s: %w", node.Name, err)
		}
		sources[id] = editorSource{Node: node, Path: path, Probe: probeMedia(ctx, path), Index: index}
	}
	return sources, nil
}

func editExportArgs(timeline TimelineData, originalName string, sources map[string]editorSource, tmpDir, preset string) (string, string, []string) {
	base := strings.TrimSuffix(filepath.Base(originalName), filepath.Ext(originalName))
	if base == "" {
		base = "video"
	}
	outName := base + "_edited.mp4"
	outPath := filepath.Join(tmpDir, outName)
	clips := videoClips(timeline)
	if canUseLosslessCut(timeline) && preset == "match" {
		clip := clips[0]
		source := sources[clip.SourceNodeID]
		if !isVideoFile(source.Node.Name, source.Node.MimeType) {
			goto render
		}
		return outName, outPath, []string{
			"-y",
			"-i", source.Path,
			"-ss", fmtSeconds(clip.StartInSource),
			"-t", fmtSeconds(clipDuration(clip)),
			"-c", "copy",
			"-movflags", "+faststart",
			outPath,
		}
	}
render:
	inputs := make([]editorSource, 0, len(sources))
	for _, source := range sources {
		inputs = append(inputs, source)
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Index < inputs[j].Index })
	args := []string{"-y"}
	for _, source := range inputs {
		args = append(args, "-i", source.Path)
	}
	var filter strings.Builder
	for i, clip := range clips {
		speed := normalizedSpeed(clip.Speed)
		source := sources[clip.SourceNodeID]
		duration := clipDuration(clip)
		videoChain := ""
		if isImageFile(source.Node.Name, source.Node.MimeType) {
			videoChain = fmt.Sprintf("scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2,setsar=1,loop=loop=-1:size=1:start=0,trim=duration=%s,setpts=PTS-STARTPTS", fmtSeconds(duration))
		} else {
			videoChain = fmt.Sprintf("trim=start=%s:end=%s,setpts=(PTS-STARTPTS)/%s,scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2,setsar=1", fmtSeconds(clip.StartInSource), fmtSeconds(clip.EndInSource), fmtFilterNumber(speed))
		}
		if preset == "1080p" {
			videoChain += ",scale=1920:1080:force_original_aspect_ratio=decrease"
		}
		filter.WriteString(fmt.Sprintf("[%d:v]%s[v%d];", source.Index, videoChain, i))
		if source.Probe.HasAudio {
			audioChain := fmt.Sprintf("atrim=start=%s:end=%s,asetpts=PTS-STARTPTS%s", fmtSeconds(clip.StartInSource), fmtSeconds(clip.EndInSource), atempoFilters(speed))
			if clip.Volume >= 0 && clip.Volume != 1 {
				audioChain += ",volume=" + fmtFilterNumber(clip.Volume)
			}
			if clip.FadeIn > 0 {
				audioChain += ",afade=t=in:st=0:d=" + fmtSeconds(clip.FadeIn)
			}
			if clip.FadeOut > 0 {
				start := clipDuration(clip) - clip.FadeOut
				audioChain += ",afade=t=out:st=" + fmtSeconds(start) + ":d=" + fmtSeconds(clip.FadeOut)
			}
			filter.WriteString(fmt.Sprintf("[%d:a]%s[a%d];", source.Index, audioChain, i))
		} else {
			filter.WriteString(fmt.Sprintf("anullsrc=channel_layout=stereo:sample_rate=44100,atrim=duration=%s[a%d];", fmtSeconds(duration), i))
		}
	}
	for i := range clips {
		filter.WriteString(fmt.Sprintf("[v%d]", i))
		filter.WriteString(fmt.Sprintf("[a%d]", i))
	}
	filter.WriteString(fmt.Sprintf("concat=n=%d:v=1:a=1[basev][basea];", len(clips)))
	audioLabels := []string{"[basea]"}
	for i, clip := range audioClips(timeline) {
		source := sources[clip.SourceNodeID]
		if !source.Probe.HasAudio {
			continue
		}
		delay := int(mathMax(0, clip.StartOnTimeline) * 1000)
		duration := mathMax(0.1, clip.EndInSource-clip.StartInSource)
		audioChain := fmt.Sprintf("atrim=start=%s:end=%s,asetpts=PTS-STARTPTS%s,volume=%s", fmtSeconds(clip.StartInSource), fmtSeconds(clip.EndInSource), atempoFilters(clip.Speed), fmtFilterNumber(mathMax(0, clip.Volume)))
		if clip.FadeIn > 0 {
			audioChain += ",afade=t=in:st=0:d=" + fmtSeconds(clip.FadeIn)
		}
		if clip.FadeOut > 0 {
			audioChain += ",afade=t=out:st=" + fmtSeconds(duration-clip.FadeOut) + ":d=" + fmtSeconds(clip.FadeOut)
		}
		audioChain += fmt.Sprintf(",adelay=%d|%d,apad=whole_dur=%s", delay, delay, fmtSeconds(timeline.Duration))
		filter.WriteString(fmt.Sprintf("[%d:a]%s[mixa%d];", source.Index, audioChain, i))
		audioLabels = append(audioLabels, fmt.Sprintf("[mixa%d]", i))
	}
	if len(audioLabels) > 1 {
		filter.WriteString(strings.Join(audioLabels, ""))
		filter.WriteString(fmt.Sprintf("amix=inputs=%d:duration=longest:dropout_transition=0[outa];", len(audioLabels)))
	} else {
		filter.WriteString("[basea]acopy[outa];")
	}
	filter.WriteString(textFilters(timeline, "[basev]", "[outv]"))

	args = append(args, "-filter_complex", filter.String(), "-map", "[outv]", "-map", "[outa]")
	switch preset {
	case "1080p":
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "20")
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	case "compressed":
		args = append(args, "-c:v", "libx265", "-preset", "medium", "-crf", "28", "-tag:v", "hvc1")
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	default:
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "18")
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	}
	args = append(args, "-movflags", "+faststart", "-pix_fmt", "yuv420p", outPath)
	return outName, outPath, args
}

func hasAudioStream(ctx context.Context, path string) bool {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=index",
		"-of", "csv=p=0",
		path,
	)
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func canUseLosslessCut(timeline TimelineData) bool {
	clips := videoClips(timeline)
	if len(clips) != 1 || hasOverlayTracks(timeline) {
		return false
	}
	clip := clips[0]
	return !clip.HasEffects && normalizedSpeed(clip.Speed) == 1
}

func hasOverlayTracks(timeline TimelineData) bool {
	for _, track := range timeline.Tracks {
		if (track.Type == "text" || track.Type == "caption") && len(track.Clips) > 0 && !track.Muted {
			return true
		}
	}
	return false
}

func clipDuration(clip TimelineClip) float64 {
	return (clip.EndInSource - clip.StartInSource) / normalizedSpeed(clip.Speed)
}

func normalizedSpeed(speed float64) float64 {
	if speed <= 0 {
		return 1
	}
	if speed < 0.1 {
		return 0.1
	}
	if speed > 16 {
		return 16
	}
	return speed
}

func atempoFilters(speed float64) string {
	speed = normalizedSpeed(speed)
	var filters []string
	for speed > 2 {
		filters = append(filters, "atempo=2")
		speed /= 2
	}
	for speed < 0.5 {
		filters = append(filters, "atempo=0.5")
		speed /= 0.5
	}
	filters = append(filters, "atempo="+fmtFilterNumber(speed))
	return "," + strings.Join(filters, ",")
}

func fmtFilterNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', 4, 64)
}

func textFilters(timeline TimelineData, input, output string) string {
	var filters []string
	for _, track := range timeline.Tracks {
		if track.Muted || (track.Type != "text" && track.Type != "caption") {
			continue
		}
		for _, clip := range track.Clips {
			if clip.Text == nil || strings.TrimSpace(clip.Text.Content) == "" {
				continue
			}
			text := *clip.Text
			size := text.FontSize
			if size <= 0 {
				size = 48
			}
			color := safeFFmpegColor(text.Color, "white")
			boxColor := safeFFmpegColor(text.BgColor, "black@0.45")
			x := "(w-text_w)/2"
			switch text.Alignment {
			case "left":
				x = fmt.Sprintf("w*%s", fmtFilterNumber(clamp01(text.X)))
			case "right":
				x = fmt.Sprintf("w*%s-text_w", fmtFilterNumber(clamp01(text.X)))
			default:
				if text.X > 0 {
					x = fmt.Sprintf("w*%s-text_w/2", fmtFilterNumber(clamp01(text.X)))
				}
			}
			y := fmt.Sprintf("h*%s-text_h/2", fmtFilterNumber(clamp01(text.Y)))
			start := clip.StartOnTimeline
			end := clip.StartOnTimeline + mathMax(0.1, clip.EndInSource-clip.StartInSource)
			fade := ""
			if text.Animation == "fade" {
				fade = ":alpha='if(lt(t," + fmtSeconds(start+0.35) + "),(t-" + fmtSeconds(start) + ")/0.35,if(gt(t," + fmtSeconds(end-0.35) + "),(" + fmtSeconds(end) + "-t)/0.35,1))'"
			}
			filters = append(filters, fmt.Sprintf("drawtext=text='%s':fontsize=%d:fontcolor=%s:box=1:boxcolor=%s:boxborderw=12:x=%s:y=%s:enable='between(t,%s,%s)'%s",
				escapeDrawtext(text.Content), size, color, boxColor, x, y, fmtSeconds(start), fmtSeconds(end), fade))
		}
	}
	if len(filters) == 0 {
		return input + "null" + output
	}
	return input + strings.Join(filters, ",") + output
}

func escapeDrawtext(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `'`, `\'`, `:`, `\:`, `%`, `\%`, "\n", `\n`, "\r", "")
	return replacer.Replace(s)
}

func safeFFmpegColor(color, fallback string) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return fallback
	}
	if strings.HasPrefix(color, "#") && (len(color) == 7 || len(color) == 9) {
		return "0x" + color[1:]
	}
	if strings.ContainsAny(color, "';[]()") {
		return fallback
	}
	return color
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func fmtSeconds(v float64) string {
	if v < 0 {
		v = 0
	}
	return fmt.Sprintf("%.3f", v)
}

func (s *Services) ProbeFile(ctx context.Context, fileID string) (FileProbe, error) {
	node, err := s.Nodes.Get(ctx, fileID)
	if err != nil {
		return FileProbe{}, err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady || (!isMediaFile(node.Name, node.MimeType) && !isImageFile(node.Name, node.MimeType)) {
		return FileProbe{}, fmt.Errorf("%w: source must be a ready media file", domain.ErrValidation)
	}
	tmpDir, err := os.MkdirTemp(s.DataDir, "probe-*")
	if err != nil {
		return FileProbe{}, err
	}
	defer os.RemoveAll(tmpDir)
	inPath := filepath.Join(tmpDir, "input"+extOf(node.Name))
	f, err := os.Create(inPath)
	if err != nil {
		return FileProbe{}, err
	}
	_, err = s.Download(ctx, node.ID, f)
	_ = f.Close()
	if err != nil {
		return FileProbe{}, err
	}
	return probeMedia(ctx, inPath), nil
}

func probeMedia(ctx context.Context, path string) FileProbe {
	probe := FileProbe{Duration: probeDuration(ctx, path), HasAudio: hasAudioStream(ctx, path)}
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "stream=codec_type,codec_name,width,height",
		"-of", "json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return probe
	}
	var payload struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return probe
	}
	for _, stream := range payload.Streams {
		switch stream.CodecType {
		case "video":
			if probe.VideoCodec == "" {
				probe.VideoCodec = stream.CodecName
				probe.Width = stream.Width
				probe.Height = stream.Height
			}
		case "audio":
			if probe.AudioCodec == "" {
				probe.AudioCodec = stream.CodecName
				probe.HasAudio = true
			}
		}
	}
	return probe
}

func (s *Services) EnsureEditorProxy(ctx context.Context, nodeID string) (string, error) {
	node, err := s.Nodes.Get(ctx, nodeID)
	if err != nil {
		return "", err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady || !isVideoFile(node.Name, node.MimeType) {
		return "", fmt.Errorf("%w: source must be a ready video file", domain.ErrValidation)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("%w: ffmpeg not installed on server", domain.ErrNotConfigured)
	}
	proxyDir := filepath.Join(s.DataDir, "editor-proxy")
	if err := os.MkdirAll(proxyDir, 0o700); err != nil {
		return "", err
	}
	proxyPath := filepath.Join(proxyDir, node.ID+".mp4")
	if st, err := os.Stat(proxyPath); err == nil && st.Size() > 0 {
		return proxyPath, nil
	}
	hub := s.proxyHub()
	hub.mu.Lock()
	job := hub.jobs[node.ID]
	if job == nil {
		job = &proxyJob{status: "generating", message: "Generating editor proxy...", path: proxyPath}
		hub.jobs[node.ID] = job
		go s.generateEditorProxy(node, proxyPath, job)
	}
	hub.mu.Unlock()
	return proxyPath, nil
}

func (s *Services) ProxyStatus(ctx context.Context, nodeID string) (ProxyStatus, error) {
	node, err := s.Nodes.Get(ctx, nodeID)
	if err != nil {
		return ProxyStatus{}, err
	}
	proxyPath := filepath.Join(s.DataDir, "editor-proxy", node.ID+".mp4")
	if st, err := os.Stat(proxyPath); err == nil && st.Size() > 0 {
		return ProxyStatus{Status: "ready", Message: "Proxy ready"}, nil
	}
	hub := s.proxyHub()
	hub.mu.Lock()
	job := hub.jobs[node.ID]
	hub.mu.Unlock()
	if job == nil {
		status := "none"
		if node.Size > 200*1024*1024 || needsProxyByName(node.Name, node.MimeType) {
			if _, err := s.EnsureEditorProxy(ctx, node.ID); err != nil {
				return ProxyStatus{}, err
			}
			status = "generating"
		}
		return ProxyStatus{Status: status}, nil
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	return ProxyStatus{Status: job.status, Message: job.message}, nil
}

func (s *Services) EditorProxyPath(ctx context.Context, nodeID string) (string, error) {
	if _, err := s.EnsureEditorProxy(ctx, nodeID); err != nil {
		return "", err
	}
	path := filepath.Join(s.DataDir, "editor-proxy", nodeID+".mp4")
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return path, nil
	}
	return "", domain.ErrNotFound
}

func (s *Services) generateEditorProxy(node domain.Node, proxyPath string, job *proxyJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	tmpDir, err := os.MkdirTemp(s.DataDir, "proxy-*")
	if err != nil {
		setProxyJob(job, "error", err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)
	inPath := filepath.Join(tmpDir, "input"+extOf(node.Name))
	f, err := os.Create(inPath)
	if err != nil {
		setProxyJob(job, "error", err.Error())
		return
	}
	_, err = s.Download(ctx, node.ID, f)
	_ = f.Close()
	if err != nil {
		setProxyJob(job, "error", "download failed: "+err.Error())
		return
	}
	tmpOut := proxyPath + ".tmp"
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y", "-i", inPath,
		"-vf", "scale='min(1280,iw)':-2",
		"-c:v", "libx264", "-preset", "fast", "-crf", "28",
		"-c:a", "aac", "-b:a", "96k",
		"-movflags", "+faststart",
		"-pix_fmt", "yuv420p",
		tmpOut,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmpOut)
		setProxyJob(job, "error", "proxy failed: "+strings.TrimSpace(string(out)))
		return
	}
	if err := os.Rename(tmpOut, proxyPath); err != nil {
		setProxyJob(job, "error", err.Error())
		return
	}
	setProxyJob(job, "ready", "Proxy ready")
}

func setProxyJob(job *proxyJob, status, message string) {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.status = status
	job.message = message
}

func needsProxyByName(name, mime string) bool {
	mime = strings.ToLower(mime)
	ext := strings.ToLower(filepath.Ext(name))
	return strings.Contains(mime, "hevc") || strings.Contains(mime, "h265") || ext == ".mov" || ext == ".mkv" || ext == ".avi"
}

func (s *Services) ProbeKeyframes(ctx context.Context, fileID string, start, end float64) ([]float64, error) {
	node, err := s.Nodes.Get(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if node.Type != domain.NodeFile || node.Status != domain.StatusReady || !isVideoFile(node.Name, node.MimeType) {
		return nil, fmt.Errorf("%w: source must be a ready video file", domain.ErrValidation)
	}
	tmpDir, err := os.MkdirTemp(s.DataDir, "keyframes-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	inPath := filepath.Join(tmpDir, "input"+extOf(node.Name))
	f, err := os.Create(inPath)
	if err != nil {
		return nil, err
	}
	_, err = s.Download(ctx, node.ID, f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	return probeKeyframes(ctx, inPath, start, end)
}

func probeKeyframes(ctx context.Context, path string, startSec, endSec float64) ([]float64, error) {
	interval := fmtSeconds(startSec) + "%"
	if endSec > startSec {
		interval += "+" + fmtSeconds(endSec-startSec)
	}
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-skip_frame", "nokey",
		"-show_entries", "frame=best_effort_timestamp_time",
		"-read_intervals", interval,
		"-of", "csv=p=0",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var frames []float64
	for _, line := range strings.Split(string(out), "\n") {
		v, err := strconv.ParseFloat(strings.TrimSpace(line), 64)
		if err == nil {
			frames = append(frames, v)
		}
	}
	return frames, nil
}

func (s *Services) ImportProjectCaptions(ctx context.Context, projectID string, r io.Reader) ([]Caption, error) {
	data, err := io.ReadAll(io.LimitReader(r, 2<<20))
	if err != nil {
		return nil, err
	}
	captions, err := ParseSRT(string(data))
	if err != nil {
		return nil, err
	}
	project, err := s.GetEditProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	timeline, err := parseTimeline(project.TimelineJSON, project.SourceNodeID, 0)
	if err != nil {
		return nil, err
	}
	timeline.Tracks = upsertCaptionTrack(timeline.Tracks, captions)
	b, err := json.Marshal(timeline)
	if err != nil {
		return nil, err
	}
	if err := s.SaveEditTimeline(ctx, projectID, string(b)); err != nil {
		return nil, err
	}
	return captions, nil
}

func (s *Services) ExportProjectCaptions(ctx context.Context, projectID string) (string, error) {
	project, err := s.GetEditProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	timeline, err := parseTimeline(project.TimelineJSON, project.SourceNodeID, 0)
	if err != nil {
		return "", err
	}
	var captions []Caption
	for _, track := range timeline.Tracks {
		if track.Type != "caption" {
			continue
		}
		for _, clip := range track.Clips {
			if clip.Text == nil || strings.TrimSpace(clip.Text.Content) == "" {
				continue
			}
			captions = append(captions, Caption{
				StartTime: clip.StartOnTimeline,
				EndTime:   clip.StartOnTimeline + mathMax(0.1, clip.EndInSource-clip.StartInSource),
				Text:      clip.Text.Content,
			})
		}
	}
	sort.Slice(captions, func(i, j int) bool { return captions[i].StartTime < captions[j].StartTime })
	for i := range captions {
		captions[i].Index = i + 1
	}
	return GenerateSRT(captions), nil
}

func upsertCaptionTrack(tracks []TimelineTrack, captions []Caption) []TimelineTrack {
	clips := make([]TimelineClip, 0, len(captions))
	for _, caption := range captions {
		clips = append(clips, TimelineClip{
			ID:              uuid.NewString(),
			StartOnTimeline: caption.StartTime,
			StartInSource:   0,
			EndInSource:     mathMax(0.1, caption.EndTime-caption.StartTime),
			Speed:           1,
			Text: &TextOverlay{
				Content:   caption.Text,
				FontSize:  34,
				Color:     "#ffffff",
				BgColor:   "rgba(0,0,0,0.55)",
				X:         0.5,
				Y:         0.86,
				Alignment: "center",
			},
		})
	}
	for i := range tracks {
		if tracks[i].Type == "caption" {
			tracks[i].Clips = clips
			return tracks
		}
	}
	return append(tracks, TimelineTrack{ID: "c1", Type: "caption", Clips: clips})
}

func isVideoFile(name, mime string) bool {
	mime = strings.ToLower(mime)
	if strings.HasPrefix(mime, "video/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".ogv":
		return true
	default:
		return false
	}
}

func isImageFile(name, mime string) bool {
	mime = strings.ToLower(mime)
	if strings.HasPrefix(mime, "image/") {
		return true
	}
	return isImageFilename(name)
}

func isVisualFile(name, mime string) bool {
	return isVideoFile(name, mime) || isImageFile(name, mime)
}
