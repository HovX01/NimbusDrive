import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  captionExportUrl,
  createEditProject,
  importCaptions,
  mediaJobStatus,
  mediaStreamUrl,
  probeFile,
  saveEditTimeline,
  saveEditTimelineBeacon,
  startMediaJob,
  type EditProject,
  type MediaJobStatus,
  type Node,
  type TimelineClip,
  type TimelineData,
  type TimelineTrack,
} from "../../api";
import { Portal } from "../Portal";
import { ExportPanel } from "./ExportPanel";
import { Inspector } from "./Inspector";
import { MediaBin } from "./MediaBin";
import { Preview } from "./Preview";
import { Timeline } from "./Timeline";
import { TextEditor } from "./TextEditor";
import { previewKind } from "../../lib/files";

type Props = {
  token: string;
  node: Node;
  onClose: () => void;
  onDone: () => void;
};

function newID() {
  return crypto.randomUUID ? crypto.randomUUID() : `clip-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function emptyTimeline(node: Node): TimelineData {
  return {
    duration: 0,
    tracks: [
      {
        id: "v1",
        type: "video",
        clips: [
          {
            id: newID(),
            sourceNodeId: node.id,
            startInSource: 0,
            endInSource: 0,
            startOnTimeline: 0,
            speed: 1,
          },
        ],
      },
    ],
  };
}

function parseTimeline(project: EditProject, node: Node): TimelineData {
  try {
    const parsed = JSON.parse(project.timeline_json) as TimelineData;
    if (parsed?.tracks?.length) return normalizeTimeline(parsed, node);
  } catch {
    /* fall through */
  }
  return emptyTimeline(node);
}

function normalizeTimeline(timeline: TimelineData, node: Node): TimelineData {
  const sourceTracks = timeline.tracks?.length ? timeline.tracks : emptyTimeline(node).tracks;
  let cursor = 0;
  const tracks: TimelineTrack[] = sourceTracks.map((track, trackIndex) => {
    const type = track.type || "video";
    const clips = [...(track.clips ?? [])].map((clip) => {
      const speed = Math.min(16, Math.max(0.1, Number(clip.speed) || 1));
      const startInSource = Math.max(0, Number(clip.startInSource) || 0);
      const sourceDuration = Math.max(0.1, Number(clip.endInSource) - startInSource || (type === "text" || type === "caption" ? 3 : 5));
      const startOnTimeline = type === "video" ? cursor : Math.max(0, Number(clip.startOnTimeline) || 0);
      if (type === "video") cursor += sourceDuration / speed;
      return {
        ...clip,
        id: clip.id || newID(),
        sourceNodeId: type === "text" || type === "caption" ? "" : clip.sourceNodeId || node.id,
        startInSource,
        endInSource: startInSource + sourceDuration,
        startOnTimeline,
        speed,
        volume: clip.volume ?? 1,
      };
    }).sort((a, b) => a.startOnTimeline - b.startOnTimeline);
    return { id: track.id || `${type}-${trackIndex + 1}`, type, muted: !!track.muted, clips };
  });
  if (!tracks.some((track) => track.type === "video")) tracks.unshift({ ...emptyTimeline(node).tracks[0], muted: false });
  const duration = tracks.reduce((max, track) => Math.max(max, ...track.clips.map((clip) => {
    const clipDuration = Math.max(0.1, clip.endInSource - clip.startInSource) / (track.type === "video" ? Math.max(0.1, clip.speed || 1) : 1);
    return clip.startOnTimeline + clipDuration;
  })), 0);
  return {
    duration,
    tracks,
  };
}

export function VideoEditor({ token, node, onClose, onDone }: Props) {
  const [project, setProject] = useState<EditProject | null>(null);
  const [timeline, setTimeline] = useState<TimelineData>(() => emptyTimeline(node));
  const [history, setHistory] = useState<TimelineData[]>([]);
  const [historyIndex, setHistoryIndex] = useState(0);
  const [activeToolPanel, setActiveToolPanel] = useState<"media" | "text" | "captions">("media");
  const [mobilePanel, setMobilePanel] = useState<"preview" | "media" | "properties" | "timeline">("timeline");
  const [selectedClipID, setSelectedClipID] = useState<string | null>(null);
  const [playhead, setPlayhead] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [duration, setDuration] = useState(0);
  const [exportOpen, setExportOpen] = useState(false);
  const [error, setError] = useState("");
  const [saveState, setSaveState] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [probeSummary, setProbeSummary] = useState("");
  const [captionBusy, setCaptionBusy] = useState(false);
  const [clipAudioJob, setClipAudioJob] = useState<MediaJobStatus | null>(null);
  const [mediaByID, setMediaByID] = useState<Record<string, Node>>({ [node.id]: node });
  const hydrated = useRef(false);
  const captionInput = useRef<HTMLInputElement>(null);
  const projectRef = useRef<EditProject | null>(null);
  const timelineRef = useRef(timeline);
  const savedJSONRef = useRef("");
  const saveTimerRef = useRef<number | null>(null);

  const clips = timeline.tracks.flatMap((track) => track.clips);
  const selectedTrack = timeline.tracks.find((track) => track.clips.some((clip) => clip.id === selectedClipID)) ?? null;
  const selectedClip = clips.find((clip) => clip.id === selectedClipID) ?? timeline.tracks.find((track) => track.type === "video")?.clips[0] ?? null;
  const selectedSourceNode = selectedClip?.sourceNodeId ? mediaByID[selectedClip.sourceNodeId] : null;
  const selectedSourceName = selectedSourceNode?.name || node.name;
  const selectedSourceKind = selectedSourceNode ? previewKind(selectedSourceNode.name, selectedSourceNode.mime_type, false) : "video";
  const canUndo = historyIndex > 0;
  const canRedo = historyIndex < history.length - 1;

  useEffect(() => {
    let cancelled = false;
    setError("");
    createEditProject(token, node.id)
      .then(({ project }) => {
        if (cancelled) return;
        const parsed = parseTimeline(project, node);
        setProject(project);
        setTimeline(parsed);
        setHistory([parsed]);
        setHistoryIndex(0);
        setSelectedClipID(parsed.tracks[0]?.clips[0]?.id ?? null);
        savedJSONRef.current = JSON.stringify(parsed);
        hydrated.current = true;
      })
      .catch((e) => {
        if (!cancelled) setError((e as Error).message);
      });
    return () => {
      cancelled = true;
    };
  }, [token, node]);

  useEffect(() => {
    if (!duration) return;
    setTimeline((current) => {
      const firstClip = current.tracks[0]?.clips[0];
      if (!firstClip || firstClip.endInSource > 0 || current.duration > 0) return current;
      const next = normalizeTimeline({
        duration,
        tracks: [{ id: "v1", type: "video", clips: [{ ...firstClip, endInSource: duration }] }],
      }, node);
      setHistory([next]);
      setHistoryIndex(0);
      return next;
    });
  }, [duration, node]);

  useEffect(() => {
    probeFile(token, node.id)
      .then((probe) => {
        const bits = [
          probe.duration ? `${probe.duration.toFixed(1)}s` : "",
          probe.width && probe.height ? `${probe.width}x${probe.height}` : "",
          probe.video_codec || "",
          probe.has_audio ? `audio ${probe.audio_codec || "yes"}` : "no audio",
        ].filter(Boolean);
        setProbeSummary(bits.join(" · "));
      })
      .catch(() => setProbeSummary(""));
  }, [node.id, token]);

  useEffect(() => {
    projectRef.current = project;
  }, [project]);

  useEffect(() => {
    if (!clipAudioJob || clipAudioJob.status === "done" || clipAudioJob.status === "error") return;
    const timer = window.setInterval(() => {
      mediaJobStatus(token, clipAudioJob.id)
        .then((status) => {
          setClipAudioJob(status);
          if (status.status === "done") onDone();
        })
        .catch((e) => setError((e as Error).message));
    }, 900);
    return () => window.clearInterval(timer);
  }, [clipAudioJob, onDone, token]);

  useEffect(() => {
    timelineRef.current = timeline;
  }, [timeline]);

  const dirty = project !== null && JSON.stringify(timeline) !== savedJSONRef.current;

  const flushSave = useCallback(async () => {
    const currentProject = projectRef.current;
    const currentTimeline = timelineRef.current;
    if (!currentProject) return true;
    const serialized = JSON.stringify(currentTimeline);
    if (serialized === savedJSONRef.current) return true;
    if (saveTimerRef.current !== null) {
      window.clearTimeout(saveTimerRef.current);
      saveTimerRef.current = null;
    }
    setSaveState("saving");
    try {
      await saveEditTimeline(token, currentProject.id, currentTimeline);
      savedJSONRef.current = serialized;
      setSaveState("saved");
      return true;
    } catch {
      setSaveState("error");
      return false;
    }
  }, [token]);

  useEffect(() => {
    if (!project || !hydrated.current) return;
    const serialized = JSON.stringify(timeline);
    if (serialized === savedJSONRef.current) return;
    setSaveState("saving");
    if (saveTimerRef.current !== null) window.clearTimeout(saveTimerRef.current);
    saveTimerRef.current = window.setTimeout(() => {
      saveTimerRef.current = null;
      void flushSave();
    }, 3000);
    return () => {
      if (saveTimerRef.current !== null) {
        window.clearTimeout(saveTimerRef.current);
        saveTimerRef.current = null;
      }
    };
  }, [flushSave, timeline, project]);

  useEffect(() => {
    function flushWithBeacon() {
      const currentProject = projectRef.current;
      const currentTimeline = timelineRef.current;
      if (!currentProject) return;
      if (JSON.stringify(currentTimeline) === savedJSONRef.current) return;
      saveEditTimelineBeacon(token, currentProject.id, currentTimeline);
    }
    window.addEventListener("beforeunload", flushWithBeacon);
    return () => {
      window.removeEventListener("beforeunload", flushWithBeacon);
      flushWithBeacon();
    };
  }, [token]);

  const requestClose = useCallback(async () => {
    if (dirty && !window.confirm("You have unsaved edits. Save and close?")) return;
    if (dirty) {
      const ok = await flushSave();
      if (!ok && !window.confirm("Autosave failed. Close anyway?")) return;
    }
    onClose();
  }, [dirty, flushSave, onClose]);

  const openExport = useCallback(async () => {
    if (dirty) {
      const ok = await flushSave();
      if (!ok) return;
    }
    setExportOpen(true);
  }, [dirty, flushSave]);

  const commitTimeline = useCallback((next: TimelineData) => {
    const normalized = normalizeTimeline(next, node);
    setTimeline(normalized);
    setHistory((prev) => {
      const base = prev.slice(0, historyIndex + 1);
      return [...base, normalized].slice(-80);
    });
    setHistoryIndex((i) => Math.min(i + 1, 79));
    setPlayhead((p) => Math.min(p, normalized.duration));
  }, [historyIndex, node]);

  const undo = useCallback(() => {
    if (!canUndo) return;
    const nextIndex = historyIndex - 1;
    setHistoryIndex(nextIndex);
    setTimeline(history[nextIndex]);
  }, [canUndo, history, historyIndex]);

  const redo = useCallback(() => {
    if (!canRedo) return;
    const nextIndex = historyIndex + 1;
    setHistoryIndex(nextIndex);
    setTimeline(history[nextIndex]);
  }, [canRedo, history, historyIndex]);

  const updateClip = useCallback((clipID: string, patch: Partial<TimelineClip>) => {
    commitTimeline({
      ...timeline,
      tracks: timeline.tracks.map((track) => ({
        ...track,
        clips: track.clips.map((clip) => (clip.id === clipID ? { ...clip, ...patch } : clip)),
      })),
    });
  }, [commitTimeline, timeline]);

  const splitSelected = useCallback(() => {
    const clip = selectedClip;
    if (!clip) return;
    const offset = playhead - clip.startOnTimeline;
    const speed = Math.max(0.1, clip.speed || 1);
    const cut = clip.startInSource + offset * speed;
    if (offset <= 0.05 || cut >= clip.endInSource - 0.05) return;
    const left = { ...clip, endInSource: cut };
    const right = {
      ...clip,
      id: newID(),
      startInSource: cut,
      startOnTimeline: clip.startOnTimeline + (cut - clip.startInSource) / speed,
    };
    commitTimeline({
      ...timeline,
      tracks: timeline.tracks.map((track) => ({
        ...track,
        clips: track.clips.flatMap((item) => (item.id === clip.id ? [left, right] : [item])),
      })),
    });
    setSelectedClipID(right.id);
  }, [commitTimeline, playhead, selectedClip, timeline]);

  const deleteSelected = useCallback(() => {
    if (!selectedClip) return;
    commitTimeline({
      ...timeline,
      tracks: timeline.tracks.map((track) => ({
        ...track,
        clips: track.clips.filter((clip) => clip.id !== selectedClip.id),
      })),
    });
    setSelectedClipID(null);
  }, [commitTimeline, selectedClip, timeline]);

  const addMediaClip = useCallback(async (media: Node) => {
    const kind = previewKind(media.name, media.mime_type, false);
    if (kind !== "video" && kind !== "image" && kind !== "audio") return;
    setMediaByID((current) => ({ ...current, [media.id]: media }));
    let mediaDuration = kind === "image" ? 5 : 10;
    try {
      const probe = await probeFile(token, media.id);
      if (probe.duration > 0) mediaDuration = probe.duration;
    } catch {
      /* keep fallback */
    }
    const trackType: TimelineTrack["type"] = kind === "audio" ? "audio" : "video";
    const clip: TimelineClip = {
      id: newID(),
      sourceNodeId: media.id,
      startInSource: 0,
      endInSource: kind === "image" ? 5 : mediaDuration,
      startOnTimeline: trackType === "audio" ? playhead : timeline.duration,
      speed: 1,
      volume: 1,
    };
    const track = timeline.tracks.find((item) => item.type === trackType);
    const nextTracks = track
      ? timeline.tracks.map((item) => item.id === track.id ? { ...item, clips: [...item.clips, clip] } : item)
      : [...timeline.tracks, { id: `${trackType}-${newID()}`, type: trackType, clips: [clip] }];
    commitTimeline({ ...timeline, tracks: nextTracks });
    setSelectedClipID(clip.id);
  }, [commitTimeline, playhead, timeline, token]);

  const addTextClip = useCallback((kind: "text" | "caption" = "text") => {
    const clip: TimelineClip = {
      id: newID(),
      sourceNodeId: "",
      startInSource: 0,
      endInSource: kind === "caption" ? 2.5 : 3,
      startOnTimeline: playhead,
      speed: 1,
      text: {
        content: kind === "caption" ? "New caption" : "New title",
        font_size: kind === "caption" ? 34 : 52,
        font_family: "Inter",
        color: "#ffffff",
        bg_color: kind === "caption" ? "rgba(0,0,0,0.55)" : "",
        x: 0.5,
        y: kind === "caption" ? 0.86 : 0.5,
        alignment: "center",
        animation: "none",
      },
    };
    const track = timeline.tracks.find((item) => item.type === kind);
    const nextTracks = track
      ? timeline.tracks.map((item) => item.id === track.id ? { ...item, clips: [...item.clips, clip] } : item)
      : [...timeline.tracks, { id: `${kind}-${newID()}`, type: kind, clips: [clip] }];
    commitTimeline({ ...timeline, tracks: nextTracks });
    setSelectedClipID(clip.id);
  }, [commitTimeline, playhead, timeline]);

  const toggleTrackMute = useCallback((trackID: string) => {
    commitTimeline({
      ...timeline,
      tracks: timeline.tracks.map((track) => track.id === trackID ? { ...track, muted: !track.muted } : track),
    });
  }, [commitTimeline, timeline]);

  const extractSelectedClipAudio = useCallback(async () => {
    if (!selectedClip?.sourceNodeId || selectedSourceKind !== "video") return;
    setError("");
    try {
      const { job_id } = await startMediaJob(token, selectedClip.sourceNodeId, "extract_audio");
      setClipAudioJob({
        id: job_id,
        action: "extract_audio",
        status: "queued",
        phase: "queued",
        progress: 0,
        message: "Extracting selected clip audio...",
      });
    } catch (e) {
      setError((e as Error).message);
    }
  }, [selectedClip?.sourceNodeId, selectedSourceKind, token]);

  async function importSRT(file: File) {
    if (!project) return;
    setCaptionBusy(true);
    try {
      await importCaptions(token, project.id, file);
      const refreshed = await createEditProject(token, node.id);
      const parsed = parseTimeline(refreshed.project, node);
      setProject(refreshed.project);
      commitTimeline(parsed);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setCaptionBusy(false);
      if (captionInput.current) captionInput.current.value = "";
    }
  }

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const tag = (e.target as HTMLElement)?.tagName;
      const typing = tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
      if (e.key === "Escape" && !exportOpen) void requestClose();
      if (typing) return;
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "z" && e.shiftKey) {
        e.preventDefault();
        redo();
      } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "z") {
        e.preventDefault();
        undo();
      } else if (e.code === "Space") {
        e.preventDefault();
        setPlaying((v) => !v);
      } else if (e.key.toLowerCase() === "s") {
        e.preventDefault();
        splitSelected();
      } else if (e.key.toLowerCase() === "t") {
        e.preventDefault();
        addTextClip("text");
      } else if (e.key === "Delete" || e.key === "Backspace") {
        e.preventDefault();
        deleteSelected();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [addTextClip, deleteSelected, exportOpen, redo, requestClose, splitSelected, undo]);

  const streamUrl = useMemo(() => mediaStreamUrl(token, node.id), [token, node.id]);

  return (
    <Portal>
      <div className={`editor-shell mobile-show-${mobilePanel}`} role="dialog" aria-modal="true" aria-label={`Edit ${node.name}`}>
        <header className="editor-toolbar">
          <button type="button" className="editor-tool-btn" onClick={() => void requestClose()} title="Back to Drive">
            <i className="fa-solid fa-arrow-left" />
          </button>
          <div className="editor-title-block">
            <span className="editor-kicker">Edit Media</span>
            <strong className="truncate">{project?.name ?? node.name}</strong>
            <span className="editor-source-meta truncate">{probeSummary || "Autosaved project · Telegram-backed media"}</span>
          </div>
          <div className="editor-toolbar-actions">
            <input
              ref={captionInput}
              type="file"
              accept=".srt,text/plain,application/x-subrip"
              hidden
              onChange={(e) => {
                const file = e.currentTarget.files?.[0];
                if (file) void importSRT(file);
              }}
            />
            <span className={`editor-save-state ${saveState}`}>{dirty ? saveState : saveState === "idle" ? "" : saveState}</span>
            <button type="button" className="editor-tool-btn" disabled={!canUndo} onClick={undo} title="Undo">
              <i className="fa-solid fa-rotate-left" />
            </button>
            <button type="button" className="editor-tool-btn" disabled={!canRedo} onClick={redo} title="Redo">
              <i className="fa-solid fa-rotate-right" />
            </button>
            <button type="button" className="editor-export-btn" onClick={() => void openExport()}>
              <i className="fa-solid fa-file-export" /> Export
            </button>
          </div>
        </header>

        {error ? (
          <div className="editor-empty">
            <p>{error}</p>
          </div>
        ) : (
          <>
            <nav className="editor-mobile-tabs" aria-label="Edit Media sections">
              <button type="button" className={mobilePanel === "preview" ? "active" : ""} onClick={() => setMobilePanel("preview")}>
                <i className="fa-solid fa-play" /> Preview
              </button>
              <button type="button" className={mobilePanel === "media" ? "active" : ""} onClick={() => setMobilePanel("media")}>
                <i className="fa-solid fa-photo-film" /> Media
              </button>
              <button type="button" className={mobilePanel === "properties" ? "active" : ""} onClick={() => setMobilePanel("properties")}>
                <i className="fa-solid fa-sliders" /> Edit
              </button>
              <button type="button" className={mobilePanel === "timeline" ? "active" : ""} onClick={() => setMobilePanel("timeline")}>
                <i className="fa-solid fa-timeline" /> Timeline
              </button>
            </nav>
            <main className="editor-main">
              <aside className="editor-assets-panel">
                <nav className="editor-mode-rail" aria-label="Editor tools">
                  <button
                    type="button"
                    className={activeToolPanel === "media" ? "active" : ""}
                    onClick={() => setActiveToolPanel("media")}
                    title="Media assets"
                  >
                    <i className="fa-solid fa-photo-film" />
                    <span>Media</span>
                  </button>
                  <button
                    type="button"
                    className={activeToolPanel === "text" ? "active" : ""}
                    onClick={() => setActiveToolPanel("text")}
                    title="Text tools"
                  >
                    <i className="fa-solid fa-font" />
                    <span>Text</span>
                  </button>
                  <button
                    type="button"
                    className={activeToolPanel === "captions" ? "active" : ""}
                    onClick={() => setActiveToolPanel("captions")}
                    title="Caption tools"
                  >
                    <i className="fa-solid fa-closed-captioning" />
                    <span>Captions</span>
                  </button>
                </nav>
                <div className="editor-tool-panel">
                  {activeToolPanel === "media" && (
                    <MediaBin token={token} parentID={node.parent_id || "root"} onAdd={(media) => void addMediaClip(media)} onChanged={onDone} />
                  )}
                  {activeToolPanel === "text" && (
                    <div className="editor-asset-toolbox">
                      <div className="editor-panel-head">
                        <div>
                          <p className="editor-panel-title">Text</p>
                          <p className="editor-muted">Add titles or labels over your video.</p>
                        </div>
                      </div>
                      <button type="button" className="editor-create-card" onClick={() => addTextClip("text")}>
                        <i className="fa-solid fa-font" />
                        <span><strong>Title text</strong><small>Add editable overlay at playhead</small></span>
                      </button>
                      <button type="button" className="editor-create-card" onClick={() => addTextClip("caption")}>
                        <i className="fa-solid fa-closed-captioning" />
                        <span><strong>Quick caption</strong><small>Add subtitle-style text</small></span>
                      </button>
                    </div>
                  )}
                  {activeToolPanel === "captions" && (
                    <div className="editor-asset-toolbox">
                      <div className="editor-panel-head">
                        <div>
                          <p className="editor-panel-title">Captions</p>
                          <p className="editor-muted">Import or export SRT subtitles.</p>
                        </div>
                      </div>
                      <button type="button" className="editor-create-card" disabled={captionBusy} onClick={() => captionInput.current?.click()}>
                        <i className="fa-solid fa-file-import" />
                        <span><strong>Import SRT</strong><small>Create caption clips from a subtitle file</small></span>
                      </button>
                      {project && (
                        <a className="editor-create-card" href={captionExportUrl(token, project.id)}>
                          <i className="fa-solid fa-file-lines" />
                          <span><strong>Export SRT</strong><small>Download captions from this project</small></span>
                        </a>
                      )}
                    </div>
                  )}
                </div>
              </aside>
              <Preview
                src={streamUrl}
                token={token}
                sourceNodeID={node.id}
                mediaByID={mediaByID}
                timeline={timeline}
                playhead={playhead}
                playing={playing}
                onPlayhead={setPlayhead}
                onPlaying={setPlaying}
                onDuration={setDuration}
              />
              {selectedTrack?.type === "text" || selectedTrack?.type === "caption" ? (
                <TextEditor clip={selectedClip} kind={selectedTrack.type} onChange={updateClip} />
              ) : (
                <Inspector
                  clip={selectedClip}
                  sourceName={selectedSourceName}
                  canExtractAudio={selectedSourceKind === "video"}
                  extractAudioJob={clipAudioJob}
                  onExtractAudio={() => void extractSelectedClipAudio()}
                  onChange={updateClip}
                />
              )}
            </main>
            <nav className="editor-quick-actions" aria-label="Editing shortcuts">
              <button
                type="button"
                className={mobilePanel === "media" ? "active" : ""}
                onClick={() => {
                  setActiveToolPanel("media");
                  setMobilePanel("media");
                }}
                title="Add media"
              >
                <i className="fa-solid fa-plus" />
                <span>Media</span>
              </button>
              <button
                type="button"
                className={mobilePanel === "properties" ? "active" : ""}
                onClick={() => setMobilePanel("properties")}
                title="Edit selected clip"
              >
                <i className="fa-solid fa-sliders" />
                <span>Edit</span>
              </button>
              <button type="button" onClick={splitSelected} disabled={!selectedClip} title="Split at playhead">
                <i className="fa-solid fa-scissors" />
                <span>Split</span>
              </button>
              <button
                type="button"
                onClick={() => {
                  addTextClip("text");
                  setMobilePanel("properties");
                }}
                title="Add text"
              >
                <i className="fa-solid fa-font" />
                <span>Text</span>
              </button>
              <button
                type="button"
                onClick={() => {
                  addTextClip("caption");
                  setMobilePanel("properties");
                }}
                title="Add captions"
              >
                <i className="fa-solid fa-closed-captioning" />
                <span>Captions</span>
              </button>
              <button
                type="button"
                onClick={() => {
                  setActiveToolPanel("captions");
                  setMobilePanel("media");
                }}
                title="Import captions"
              >
                <i className="fa-solid fa-file-import" />
                <span>Import</span>
              </button>
              <button
                type="button"
                onClick={() => void extractSelectedClipAudio()}
                disabled={!selectedClip?.sourceNodeId || selectedSourceKind !== "video"}
                title="Extract audio"
              >
                <i className="fa-solid fa-music" />
                <span>Audio</span>
              </button>
              <button type="button" onClick={deleteSelected} disabled={!selectedClip} title="Delete selected clip">
                <i className="fa-solid fa-trash" />
                <span>Delete</span>
              </button>
              <button type="button" className="primary" onClick={() => void openExport()} title="Export video">
                <i className="fa-solid fa-file-export" />
                <span>Export</span>
              </button>
            </nav>
            <div className="editor-timeline-wrap">
              <Timeline
                timeline={timeline}
                selectedClipID={selectedClipID}
                playhead={playhead}
                onSelectClip={setSelectedClipID}
                onPlayhead={setPlayhead}
                onChange={commitTimeline}
                onToggleTrackMute={toggleTrackMute}
                onSplit={splitSelected}
                onDelete={deleteSelected}
              />
            </div>
          </>
        )}

        {exportOpen && project && (
          <ExportPanel
            token={token}
            project={project}
            onClose={() => setExportOpen(false)}
            onDone={onDone}
          />
        )}
      </div>
    </Portal>
  );
}
