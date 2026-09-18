# Nimbus — Full Video Editor (in-app)

Implement a **full video editing feature inside Nimbus** (`cloud_storage`).

## What Nimbus already is

Nimbus is a **self-hosted personal Drive**:

| Layer | Reality in this repo |
|--------|----------------------|
| Product name | **Nimbus** |
| Backend | **Go** — `cmd/nimbus`, `internal/app`, `internal/http`, `internal/store/sqlite`, `internal/telegram` |
| Frontend | **React + TypeScript + Vite** — `web/src` |
| Auth | Telegram MTProto login → JWT (`/api/v1/auth/*`) |
| Metadata | **SQLite** (`nodes`, `file_parts`, shares, bots, …) under `DATA_DIR` |
| File bytes | **Telegram channel** via `BlobStore` / `UploadPart` / Range download — **not** local disk as source of truth |
| Local caches only | thumbs (`EnsureThumb`), mediacache, fetch-tmp, upload-tmp |
| Deploy | Docker Compose + Caddy at `nimbus.home.arpa` (`deploy/compose.yml`, `deploy/sync.ps1`) |
| FFmpeg | Already in the runtime image (`Dockerfile` installs `ffmpeg`) |

Existing media workflow to extend (do **not** replace):

- Drive UI: `web/src/components/DriveShell.tsx`, `DriveCard.tsx`, `PreviewModal.tsx`
- One-shot FFmpeg jobs: **Media Studio** — `MediaStudioModal.tsx` + `POST /api/v1/files/{id}/media` + `GET /api/v1/files/media/{jobID}` (`internal/app/media.go`)
  - Actions today: `playable` / `enhance` / `compress`
- Async job pattern also used by Fetch: `internal/app/fetch.go` (queued → running → done/error + progress)
- Upload / dedupe: `Services.Upload` (SHA-256 content hash)
- Video preview / Range streaming: download + mediacache paths

**References** (concepts / UI / lossless / captions — adapt to Go + React, do not adopt their stacks wholesale):

- OpenCut — https://github.com/OpenCut-app/OpenCut  
- OpenCut Classic — https://github.com/OpenCut-app/opencut-classic  
- Kdenlive — https://github.com/KDE/kdenlive  
- Shotcut — https://github.com/mltframework/shotcut  
- MLT — https://github.com/mltframework/mlt  
- OpenShot / libopenshot — https://github.com/OpenShot/openshot-qt · https://github.com/OpenShot/libopenshot  
- LosslessCut — https://github.com/mifi/lossless-cut  
- Sthang Studio (AI / Khmer captions) — https://github.com/Sthang-Co-Ltd/Sthang-Studio  

## Hard constraints

* This is **not** a new app. No separate repo, no Electron shell, no replacing Go/React/SQLite/Telegram.
* Do **not** invent a second auth, upload, or blob store.
* Do **not** store master media only on disk — originals and exports must go through **existing** `Upload` → Telegram + SQLite index.
* Never mutate the original Drive file; exports are **new** nodes (same parent folder by default).
* Follow existing naming, error types (`domain.Err*`), JWT/`X-Nimbus-Access` auth, and Docker deploy path.
* Prefer extending Media Studio / media jobs over a parallel “ffmpeg microservice”.
* Self-hosted: must work behind Caddy TLS and on LAN/Tailscale like the rest of Nimbus.
* After changes: run relevant `go test` / web build; fix regressions. Deploy via `deploy/sync.ps1` only when asked.

## Purpose / UX flow

User selects a video in Drive → **Edit Video** → in-app CapCut-style editor → Save project / Export → result appears as a new file in Drive.

```
Drive (node id)
  → Edit Video
  → Video Editor (project autosaved)
  → Export job (FFmpeg on server)
  → New node via Services.Upload (Telegram bytes + SQLite metadata)
```

Entry points:

1. Drive card / list actions (next to Media Studio / Share / Send Telegram)
2. Preview modal actions
3. Optional: deep link `/edit/:fileId` or modal full-screen editor route inside the SPA

## Editor UI (CapCut-style)

Build inside `web/` (new components under e.g. `web/src/components/editor/`), matching Nimbus light Drive visual language where possible:

* Media panel (bin) — pull additional Drive media by file id, not only local blobs
* Video preview (use existing auth’d download / Range URLs; proxies when available)
* Multi-track timeline (video / audio / image / text / caption)
* Properties / inspector
* Export controls
* Timeline zoom, playhead, frame seek
* Drag/drop clips, trim, split, delete, duplicate, reorder
* Undo / redo

Transforms & effects:

* Crop, resize, rotate, position, opacity
* Speed, reverse, freeze frame
* Transitions, filters, LUTs, basic color
* Keyframes
* Text + animated text
* Audio volume, fade in/out, waveform

## Lossless editing

Study LosslessCut. For trim/split/remux, prefer **stream copy** (no re-encode) when possible.

Server (or client planner) must decide:

* lossless path → FFmpeg `-c copy` (or equivalent)
* vs full render → encode

Never silently destroy quality for “simple cut” jobs.

## AI captions + Khmer

Study Sthang Studio. Integrate the strongest caption ideas into **this** editor:

* AI transcription (Whisper or compatible CLI/service runnable in Docker)
* Khmer transcription + editing
* Forced alignment, Whisper fallback
* Caption timing on the timeline, waveform-assisted timing
* Correction memory / locks
* Caption styling, SRT import/export, burned-in burn-in on export
* Cached + resumable jobs (same async job status model as Media Studio)

Captions live on the timeline; burn-in is an export option.

## Cloud / Telegram integration (critical)

Media identity in Nimbus is a **file node id** (`nodes.id`), not a local path.

Editor must use existing:

* Auth JWT (+ access key if required)
* `GET /api/v1/files/{id}/download` (and Range / mediacache behavior)
* Thumbs: `GET /api/v1/files/{id}/thumb`
* Upload: `POST /api/v1/files/upload` or internal `Services.Upload`
* Parent folder = current Drive folder / source file’s `parent_id`

Project persistence example:

* Original: `holiday.mov` (unchanged Telegram-backed node)
* Project: DB row + JSON (or SQLite blob) e.g. `holiday — edit project` referencing `source_node_id`
* Export: `holiday-edited.mp4` as a **new** ready file node

Projects should store **node ids** for source clips, not copied bytes, unless a proxy derivative is generated.

## Server-side jobs

Reuse the Media Studio / Fetch job pattern (`queued | running | done | error`, progress %, message):

Heavy work as background jobs:

* proxy generation
* waveform generation
* thumbnails / scrub sprites (optional)
* transcription / alignment
* lossless cut / full render / burn-in export

Suggested API shape (adapt to existing router style in `internal/http/server.go`):

* `POST /api/v1/files/{id}/edit/projects` — create project from Drive video
* `GET/PATCH /api/v1/edit/projects/{projectId}` — load / autosave timeline JSON
* `POST /api/v1/edit/projects/{projectId}/export` — start export job
* `GET /api/v1/edit/jobs/{jobId}` — progress (mirror media job status)

Do not block Drive list/upload/fetch while rendering.

Optional: reuse/extend `internal/app/media.go` hub instead of a third job map if that stays clearer.

## Formats & compatibility

FFmpeg is already in the container. Prefer it over new native deps.

Support at least: MP4, MOV, MKV, WebM · H.264, HEVC/H.265, VP9, AV1, ProRes where FFmpeg allows.

**iPhone MOV/HEVC** matters — Media Studio already has `playable` for browser-safe H.264; editor preview may need a proxy even when the master is HEVC.

Do not needlessly downscale masters on export; honor user export presets (match / 1080p / compressed).

## Proxy editing

For large / high-bitrate masters:

* Preview & timeline scrub → lightweight **proxy** (local under `DATA_DIR`, or a derivative node marked as proxy)
* Final export → always from **original** Telegram-backed bytes (download via existing blob path), never from proxy

## Persistence

Editing projects must survive refresh, re-login, container restart, and failed renders.

* Autosave timeline JSON to SQLite (new tables OK; migrate in `internal/store/sqlite`)
* User can leave editor and resume later from Drive (“Edit projects” or reopen via action on the project entry)

## Implementation order

Do **not** only write an architecture doc. Inspect the repo, then implement.

1. Trace `nodes` / `file_parts` / `Upload` / download / PreviewModal / Media Studio jobs.
2. Add **Edit Video** on video files (Drive + Preview); keep Media Studio for one-shot enhance/compress/playable.
3. Editor shell route/modal; load source by **file id** with authenticated stream/proxy.
4. Preview + single-track timeline: trim / split / undo.
5. Project create + autosave in SQLite.
6. Export job → FFmpeg → `Services.Upload` → new Drive file + toast / refresh list.
7. Multi-track, text, audio, transitions.
8. Proxy pipeline for HEVC/large files.
9. Captions (SRT first, then AI/Khmer).
10. Lossless path for simple cuts when possible.

## Explicit non-goals

* Replacing Telegram storage with S3/local-only masters
* Porting OpenCut/Next.js or Shotcut/Qt into this repo as the UI framework
* Breaking Fetch, Share, Send to Telegram, Media Studio, or PWA share-target
* Committing secrets (`.env`, Telegram session)

## Start now

Inspect the codebase first, then implement starting at step 1–6 above. Reuse Nimbus components and services. Ship incremental, working slices that deploy with the existing Docker setup.
