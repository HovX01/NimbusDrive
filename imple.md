# Nimbus Video Editor — Phase 2 Implementation Plan (Steps 7–10 + Phase 1 Fixes)

## Current State After Phase 1

Working single-track video editor with:
- ✅ Edit Video entry points (DriveCard, list view, PreviewModal)
- ✅ Full-screen editor shell with Portal
- ✅ Preview player with authenticated streaming
- ✅ Single-track timeline: trim, split, delete, reorder clips
- ✅ Undo/redo (80-state history)
- ✅ Autosave to SQLite (3s debounce)
- ✅ Export: lossless (`-c copy`) for simple trims, full re-encode for multi-clip/presets
- ✅ Export progress polling, result uploaded as new Drive node

### Phase 1 Gaps to Fix First

| # | Issue | File | Fix |
|---|-------|------|-----|
| G1 | **Job hub memory leak** — finished export jobs never evicted | [`editor.go`](file:///d:/cloud_storage/internal/app/editor.go) | Add TTL-based eviction: purge `done`/`error` jobs older than 30 min on each `StartEditExport` call |
| G2 | **Multi-clip playback discontinuity** — video plays through trimmed-out gaps | [`Preview.tsx`](file:///d:/cloud_storage/web/src/components/editor/Preview.tsx) | On `timeupdate`, if current source time exits active clip's range, seek `<video>` to next clip's `startInSource`; if no next clip, pause |
| G3 | **Autosave lost on close** — debounce cancelled without flushing | [`VideoEditor.tsx`](file:///d:/cloud_storage/web/src/components/editor/VideoEditor.tsx) | Add `useEffect` cleanup that flushes pending save synchronously (or via `navigator.sendBeacon`); add "Unsaved changes" confirmation if dirty |
| G4 | **Speed changes not in FFmpeg** — `TimelineClip.Speed` parsed but ignored | [`editor.go`](file:///d:/cloud_storage/internal/app/editor.go) | Wire `setpts=PTS/{speed}` for video and `atempo={speed}` for audio in `editExportArgs` filter complex |
| G5 | **Lossless cut keyframe drift** — `-ss` before `-i -c copy` cuts at nearest keyframe | [`editor.go`](file:///d:/cloud_storage/internal/app/editor.go) | Use `-ss` after `-i` for frame-accurate start (slower but precise); add `canUseLosslessCut` heuristic to warn user |

---

## Phase 2A: Multi-Track Timeline (Step 7)

### Backend Changes

#### [MODIFY] [`internal/app/editor.go`](file:///d:/cloud_storage/internal/app/editor.go)

Extend the timeline data model and FFmpeg pipeline:

**Timeline model expansion:**
```go
type TimelineTrack struct {
    ID    string         `json:"id"`
    Type  string         `json:"type"`   // "video" | "audio" | "text" | "image"
    Clips []TimelineClip `json:"clips"`
    Muted bool           `json:"muted"`  // NEW: per-track mute
}

type TimelineClip struct {
    // ... existing fields ...
    Volume    float64        `json:"volume"`     // 0.0–1.0 (audio)
    FadeIn    float64        `json:"fade_in"`    // seconds
    FadeOut   float64        `json:"fade_out"`   // seconds
    Text      *TextOverlay   `json:"text,omitempty"`
    Transition *Transition   `json:"transition,omitempty"`
}

type TextOverlay struct {
    Content    string  `json:"content"`
    FontSize   int     `json:"font_size"`
    FontFamily string  `json:"font_family"`   // "Inter", "Noto Sans Khmer", etc.
    Color      string  `json:"color"`         // hex
    BgColor    string  `json:"bg_color"`      // hex, optional
    X          float64 `json:"x"`             // 0.0–1.0 relative position
    Y          float64 `json:"y"`
    Alignment  string  `json:"alignment"`     // "center" | "left" | "right"
    Animation  string  `json:"animation"`     // "none" | "fade" | "slide-up" | "typewriter"
}

type Transition struct {
    Type     string  `json:"type"`     // "crossfade" | "fade-black" | "wipe-left"
    Duration float64 `json:"duration"` // seconds
}
```

**FFmpeg pipeline expansion:**
- Multi-video track: `overlay` filter to composite tracks (lower track = base, upper tracks overlaid)
- Text track: `drawtext` filter with font file, position, timing, and optional fade
- Audio track: `amix` filter to mix multiple audio inputs; `volume` filter for per-clip levels; `afade` for fade in/out
- Transitions: `xfade` filter between consecutive clips (crossfade, fade, wipeleft, etc.)
- Speed: `setpts=PTS/{speed}` + `atempo={speed}` (chain multiple `atempo` for >2x or <0.5x)

#### [MODIFY] [`internal/http/editor_handlers.go`](file:///d:/cloud_storage/internal/http/editor_handlers.go)

Add endpoint for adding media from Drive to the editor timeline:

```go
// Probe a Drive file for use in the editor (get duration, codecs, dimensions)
r.Get("/api/v1/files/{id}/probe", s.probeFile)
```

Response: `{ duration, width, height, video_codec, audio_codec, has_audio }` — needed for the frontend to place clips from additional Drive files with correct duration.

---

### Frontend Changes

#### [MODIFY] [`web/src/components/editor/Timeline.tsx`](file:///d:/cloud_storage/web/src/components/editor/Timeline.tsx)

Major expansion (~300 lines → ~600 lines):

- **Multi-track rendering**: Stack tracks vertically (video tracks on top, audio below, text at bottom)
- **Track headers**: Left sidebar showing track type icon, name, mute/solo buttons, lock toggle
- **Track management**: "Add track" button (video, audio, text), delete empty tracks, drag to reorder
- **Visual differentiation**: Video clips = blue, audio clips = green, text clips = purple
- **Audio waveform placeholder**: Colored bar with volume indicator (actual waveform in Phase 2C)
- **Transition indicators**: Diamond/chevron markers between clips where transitions exist
- **Clip drag between tracks**: Allow moving clips between compatible tracks

#### [NEW] `web/src/components/editor/TextEditor.tsx` (~120 lines)

Inline text editing panel (replaces Inspector when a text clip is selected):
- Content textarea
- Font family dropdown (Inter, system fonts, Noto Sans Khmer for Khmer support)
- Font size slider (12–200px)
- Color picker (text + background)
- Position presets: center, top-center, bottom-center, custom X/Y
- Animation dropdown: none, fade, slide-up, typewriter
- Live preview overlay on the video

#### [NEW] `web/src/components/editor/MediaBin.tsx` (~100 lines)

Media panel (source clips from Drive):
- Shows source video thumbnail + name
- "Add from Drive" button → mini file picker (reuse the pattern from `MoveModal.tsx` folder browser)
- Drag media from bin onto timeline to add clips
- Shows duration, resolution, codec info (from `/probe` endpoint)

#### [MODIFY] [`web/src/components/editor/VideoEditor.tsx`](file:///d:/cloud_storage/web/src/components/editor/VideoEditor.tsx)

- Layout expansion: add collapsible left sidebar for MediaBin
- Track management: add/remove tracks, normalize multi-track timeline
- Text clip creation: toolbar button "Add Text" creates text clip on text track
- Inspector routing: show TextEditor for text clips, Inspector for video/audio clips
- Update keyboard shortcuts: `T` to add text clip at playhead

#### [MODIFY] [`web/src/components/editor/Preview.tsx`](file:///d:/cloud_storage/web/src/components/editor/Preview.tsx)

- **Text overlay rendering**: Draw text clips as positioned `<div>` overlays on top of `<video>` element
- **Multi-source handling**: If timeline references multiple source files, manage multiple `<video>` elements (hidden) and composite via canvas, or switch visible source based on active clip

#### [MODIFY] [`web/src/api.ts`](file:///d:/cloud_storage/web/src/api.ts)

Add:
```ts
export type FileProbe = {
  duration: number; width: number; height: number;
  video_codec: string; audio_codec: string; has_audio: boolean;
};
export async function probeFile(token: string, fileId: string): Promise<FileProbe> { ... }
```

#### [MODIFY] [`web/src/styles.css`](file:///d:/cloud_storage/web/src/styles.css)

Add ~150 lines for:
- Multi-track layout (`.timeline-track-header`, `.track-controls`, `.track-mute`, `.track-label`)
- Track type colors (`.timeline-clip.video`, `.timeline-clip.audio`, `.timeline-clip.text`)
- Text editor panel (`.text-editor`, `.text-preview-overlay`)
- Media bin sidebar (`.media-bin`, `.media-bin-item`)
- Transition markers (`.transition-marker`)

---

## Phase 2B: Proxy Pipeline (Step 8)

### Backend Changes

#### [MODIFY] [`internal/app/editor.go`](file:///d:/cloud_storage/internal/app/editor.go)

Add proxy generation job (reuses `editExportHub` or separate `proxyHub`):

```go
func (s *Services) EnsureEditorProxy(ctx context.Context, nodeID string) (proxyPath string, error) {
    // 1. Check if proxy already exists at DataDir/editor-proxy/<nodeID>.mp4
    // 2. If not, download source from Telegram
    // 3. Transcode: ffmpeg -i source -c:v libx264 -crf 28 -preset fast
    //    -vf "scale='min(1280,iw)':-2" -c:a aac -b:a 96k -movflags +faststart proxy.mp4
    // 4. Return local proxy path
}
```

New HTTP endpoint:
```go
// Serve proxy for editor preview (lower quality, fast seeking)
r.Get("/api/v1/files/{id}/proxy", s.serveEditorProxy)
```

Auto-trigger logic: when `CreateEditProject` is called for a file that is:
- HEVC/H.265 (browser can't natively play)
- > 200 MB (too large for smooth scrubbing)
- ProRes/AV1 or other exotic codec

Start proxy generation as a background job and serve it via `/proxy` endpoint once ready.

#### [MODIFY] [`internal/http/editor_handlers.go`](file:///d:/cloud_storage/internal/http/editor_handlers.go)

```go
r.Get("/api/v1/files/{id}/proxy", s.serveEditorProxy)    // stream proxy file
r.Get("/api/v1/files/{id}/proxy/status", s.proxyStatus)   // "none" | "generating" | "ready"
```

### Frontend Changes

#### [MODIFY] [`web/src/components/editor/Preview.tsx`](file:///d:/cloud_storage/web/src/components/editor/Preview.tsx)

- On editor mount, check proxy status: `GET /api/v1/files/{id}/proxy/status`
- If `ready`: use proxy URL for `<video src>` (faster seeking, guaranteed H.264)
- If `generating`: show "Generating preview proxy…" indicator, poll until ready
- If `none`: use original `mediaStreamUrl` (current behavior)
- Badge in corner: "Proxy" or "Original" to indicate quality mode

#### [MODIFY] [`web/src/api.ts`](file:///d:/cloud_storage/web/src/api.ts)

```ts
export function proxyStreamUrl(token: string, id: string): string { ... }
export async function getProxyStatus(token: string, id: string): Promise<"none" | "generating" | "ready"> { ... }
```

---

## Phase 2C: Captions (Step 9)

### Backend Changes

#### [NEW] [`internal/app/captions.go`](file:///d:/cloud_storage/internal/app/captions.go) (~300 lines)

**SRT parsing & generation:**
```go
func ParseSRT(data string) ([]Caption, error)
func GenerateSRT(captions []Caption) string

type Caption struct {
    Index     int     `json:"index"`
    StartTime float64 `json:"start_time"` // seconds
    EndTime   float64 `json:"end_time"`
    Text      string  `json:"text"`
}
```

**AI transcription job** (async, same hub pattern):
```go
type TranscribeJobStatus struct {
    ID       string    `json:"id"`
    Status   string    `json:"status"`  // queued | running | done | error
    Phase    string    `json:"phase"`   // download | transcribe | align | done
    Progress float64   `json:"progress"`
    Message  string    `json:"message"`
    Captions []Caption `json:"captions,omitempty"`
}

func (s *Services) StartTranscribe(ctx context.Context, nodeID string, lang string) (jobID string, err error) {
    // 1. Download source audio (extract with ffmpeg -vn -acodec pcm_s16le -ar 16000 audio.wav)
    // 2. Run Whisper CLI: whisper --model medium --language <lang> --output_format json audio.wav
    //    Or use whisper.cpp for lower resource usage
    // 3. Parse word-level timestamps from Whisper JSON output
    // 4. Return captions with precise timing
}
```

**Caption burn-in on export:**
- Add `Captions []Caption` and `BurnInCaptions bool` to export request
- If burn-in enabled: add `subtitles` filter or `drawtext` chain to FFmpeg command
- Alternatively: generate `.srt` temp file and use `subtitles=srt_file` filter

**Khmer support considerations:**
- Whisper `medium` or `large` model supports Khmer (`km`)
- Font: bundle Noto Sans Khmer in Docker image, or use `fontconfig` to locate system fonts
- Dockerfile: `RUN apk add --no-cache font-noto-khmer` (or download from Google Fonts)

#### [MODIFY] Dockerfile

```dockerfile
# Add Whisper dependencies (Python + model)
RUN apk add --no-cache python3 py3-pip
RUN pip3 install openai-whisper
# OR: build whisper.cpp from source for lower resource usage

# Add Khmer font
RUN apk add --no-cache font-noto font-noto-extra
```

#### [MODIFY] [`internal/http/editor_handlers.go`](file:///d:/cloud_storage/internal/http/editor_handlers.go)

New endpoints:
```go
r.Post("/api/v1/files/{id}/transcribe", s.startTranscribe)        // start AI transcription
r.Get("/api/v1/edit/transcribe/{jobId}", s.transcribeStatus)       // poll status
r.Post("/api/v1/edit/projects/{projectId}/captions/import", s.importCaptions)  // upload SRT
r.Get("/api/v1/edit/projects/{projectId}/captions/export", s.exportCaptions)   // download SRT
```

### Frontend Changes

#### [NEW] `web/src/components/editor/CaptionEditor.tsx` (~250 lines)

- **Caption list panel**: Scrollable list of timed captions, click to jump to time
- **Inline editing**: Click caption text to edit, adjust start/end times
- **Add/delete captions**: Insert at playhead, delete selected
- **Waveform-assisted timing**: Visual audio waveform behind caption timing bars (if waveform data available)
- **Import SRT button**: File picker → upload → parse and populate timeline
- **Export SRT button**: Download current captions as `.srt` file
- **AI Transcribe button**: Trigger transcription job with language selector (English, Khmer, auto-detect)
- **Correction memory**: Mark captions as "locked" (won't be overwritten by re-transcription)

#### [MODIFY] [`web/src/components/editor/Timeline.tsx`](file:///d:/cloud_storage/web/src/components/editor/Timeline.tsx)

- Add caption track type: captions rendered as small labeled blocks on a dedicated track
- Caption blocks show first few words, colored by confidence (if available from Whisper)
- Drag caption edges to adjust timing

#### [MODIFY] [`web/src/components/editor/Preview.tsx`](file:///d:/cloud_storage/web/src/components/editor/Preview.tsx)

- Render active captions as styled overlays at bottom of video preview
- Support caption styling (font size, color, background, position)

#### [MODIFY] [`web/src/components/editor/ExportPanel.tsx`](file:///d:/cloud_storage/web/src/components/editor/ExportPanel.tsx)

- Add "Burn-in captions" checkbox
- Add "Export SRT alongside" checkbox

---

## Phase 2D: Lossless Refinement (Step 10)

### Backend Changes

#### [MODIFY] [`internal/app/editor.go`](file:///d:/cloud_storage/internal/app/editor.go)

**Smart rendering** — hybrid lossless + minimal re-encode:

```go
func (s *Services) smartRenderExport(job *editExportJob, inPath, outPath string, timeline TimelineData) error {
    // For each clip in the timeline:
    //   1. If clip has no effects, speed=1, and is keyframe-aligned:
    //      → Extract segment with -c copy (lossless)
    //   2. If clip has effects, speed change, or non-keyframe boundaries:
    //      → Re-encode only that segment
    // 3. Concatenate all segments using concat demuxer
    // Result: minimal quality loss, maximum speed
}
```

**Keyframe index query:**
```go
func probeKeyframes(ctx context.Context, path string, startSec, endSec float64) ([]float64, error) {
    // ffprobe -select_streams v:0 -show_entries packet=pts_time,flags
    //   -read_intervals {start}%{end} -of csv=p=0 path
    // Filter for flags containing "K" (keyframe)
    // Return list of keyframe timestamps in seconds
}
```

#### [MODIFY] [`internal/http/editor_handlers.go`](file:///d:/cloud_storage/internal/http/editor_handlers.go)

```go
r.Get("/api/v1/files/{id}/keyframes", s.getKeyframes)  // query params: start, end
```

### Frontend Changes

#### [MODIFY] [`web/src/components/editor/Timeline.tsx`](file:///d:/cloud_storage/web/src/components/editor/Timeline.tsx)

- **Keyframe markers**: Small triangles on the time ruler at keyframe positions (fetched from `/keyframes`)
- **Snap-to-keyframe**: When dragging trim handles, snap to nearest keyframe with visual indicator
- **Lossless indicator**: Badge on clip showing "Lossless" (green) or "Re-encode" (yellow) based on whether cut points align with keyframes and no effects are applied

#### [MODIFY] [`web/src/components/editor/ExportPanel.tsx`](file:///d:/cloud_storage/web/src/components/editor/ExportPanel.tsx)

- Show summary: "3 segments lossless, 1 segment re-encoded" 
- "Smart render" toggle (default on for `match` preset)

---

## Implementation Order

| Priority | Phase | Scope | Est. Effort |
|----------|-------|-------|-------------|
| **1** | **1.5** | Fix G1–G5 (job eviction, playback, autosave flush, speed, keyframe) | Small |
| **2** | **2B** | Proxy pipeline (enables editing HEVC / large files) | Medium |
| **3** | **2A** | Multi-track, text overlays, audio, transitions | Large |
| **4** | **2C** | Captions (SRT → manual → AI/Whisper → Khmer) | Large |
| **5** | **2D** | Lossless refinement (keyframe snapping, smart render) | Medium |

> [!TIP]
> **Recommended start**: Phase 1.5 fixes (quick wins, improves existing UX) → Phase 2B proxy (unblocks HEVC editing) → Phase 2A multi-track (biggest feature value).

---

## Open Questions

> [!IMPORTANT]
> **Whisper hosting**: Whisper requires ~2–4 GB RAM for the `medium` model. Should we:
> - (A) Bundle `whisper.cpp` in the Docker image (lighter, C++, ~1.5 GB for medium model)
> - (B) Use Python `openai-whisper` (easier but heavier)
> - (C) Support an external Whisper API endpoint (user-configured, offloads compute)
> - (D) Defer AI transcription entirely and focus on SRT import + manual caption editing first

> [!IMPORTANT]
> **Transition library scope**: How many transitions should Phase 2A include? Recommendation:
> - Minimum: `crossfade`, `fade-to-black` (2 transitions, high impact, simple FFmpeg `xfade`)
> - Medium: + `wipe-left`, `wipe-right`, `slide-up`, `slide-down` (6 total)
> - Full: CapCut-style library with 20+ effects (significant effort, lower priority)

> [!IMPORTANT]
> **Multi-source video composition**: Phase 1 enforces single source. Should multi-track allow:
> - (A) Multiple Drive videos on separate tracks (picture-in-picture, cutaway) — requires multi-source download + complex overlay filter
> - (B) Same source on video track + separate audio/text tracks only — simpler, still powerful
> - (C) Full multi-source from the start

---

## Verification Plan

### Phase 1.5 Verification
```bash
# Backend tests
cd d:\cloud_storage && go test ./internal/app/... -v -run TestEdit
# Frontend build
cd d:\cloud_storage\web && npx tsc --noEmit && npm run build
```

### Manual Testing Checklist
- [ ] Export 5+ videos → check job hub memory doesn't grow unbounded
- [ ] Split a video into 3 clips → play through → verify seamless clip transitions
- [ ] Edit timeline → immediately close → reopen → verify last edit was saved
- [ ] Set clip speed to 2x → export → verify output is half duration
- [ ] Edit an HEVC .mov file → verify proxy auto-generates → smooth preview
- [ ] Import SRT → captions appear on timeline → export with burn-in → verify
- [ ] Trim video at non-keyframe → export as "match" → verify no frozen frames
