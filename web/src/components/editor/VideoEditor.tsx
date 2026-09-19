import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  Captions,
  FileInput,
  FileText,
  Film,
  Music,
  Plus,
  Redo2,
  Scissors,
  Share,
  SlidersHorizontal,
  Trash2,
  Type,
  Undo2,
} from "lucide-react";
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
import { ExportPanel } from "./ExportPanel";import { Inspector } from "./Inspector";
import { MediaBin } from "./MediaBin";
import { Preview } from "./Preview";
import { Timeline } from "./Timeline";
import { TextEditor } from "./TextEditor";
import { previewKind } from "../../lib/files";
import { cn } from "@/lib/utils";
import { Alert, AlertDescription } from "../ui/alert";
import { Button } from "../ui/button";

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

  const railItems: { id: "media" | "text" | "captions"; label: string; icon: typeof Film }[] = [
    { id: "media", label: "Media", icon: Film },
    { id: "text", label: "Text", icon: Type },
    { id: "captions", label: "Captions", icon: Captions },
  ];

  const quickActions: { label: string; icon: typeof Film; onClick: () => void; disabled?: boolean; active?: boolean; primary?: boolean; title: string }[] = [
    { label: "Media", icon: Plus, title: "Add media", active: mobilePanel === "media", onClick: () => { setActiveToolPanel("media"); setMobilePanel("media"); } },
    { label: "Edit", icon: SlidersHorizontal, title: "Edit selected clip", active: mobilePanel === "properties", onClick: () => setMobilePanel("properties") },
    { label: "Split", icon: Scissors, title: "Split at playhead", disabled: !selectedClip, onClick: splitSelected },
    { label: "Text", icon: Type, title: "Add text", onClick: () => { addTextClip("text"); setMobilePanel("properties"); } },
    { label: "Captions", icon: Captions, title: "Add captions", onClick: () => { addTextClip("caption"); setMobilePanel("properties"); } },
    { label: "Import", icon: FileInput, title: "Import captions", onClick: () => { setActiveToolPanel("captions"); setMobilePanel("media"); } },
    { label: "Audio", icon: Music, title: "Extract audio", disabled: !selectedClip?.sourceNodeId || selectedSourceKind !== "video", onClick: () => void extractSelectedClipAudio() },
    { label: "Delete", icon: Trash2, title: "Delete selected clip", disabled: !selectedClip, onClick: deleteSelected },
    { label: "Export", icon: Share, title: "Export video", primary: true, onClick: () => void openExport() },
  ];

  return (
    <Portal>
      <div
        className="fixed inset-0 z-50 flex min-h-0 flex-col bg-background text-foreground"
        role="dialog"
        aria-modal="true"
        aria-label={`Edit ${node.name}`}
      >
        <header className="flex shrink-0 items-center gap-3 border-b bg-card px-3 py-2.5 sm:px-4">
          <Button type="button" variant="ghost" size="icon-sm" onClick={() => void requestClose()} title="Back to Drive">
            <ArrowLeft />
          </Button>
          <div className="grid min-w-0 flex-1 gap-0.5">
            <span className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Edit Media</span>
            <strong className="truncate text-sm font-semibold leading-tight">{project?.name ?? node.name}</strong>
            <span className="truncate text-xs text-muted-foreground">{probeSummary || "Autosaved project · Telegram-backed media"}</span>
          </div>
          <div className="flex min-w-0 items-center gap-1.5">
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
            {dirty && (
              <span
                className={cn(
                  "hidden text-xs font-medium capitalize sm:inline",
                  saveState === "error" ? "text-destructive" : "text-muted-foreground",
                )}
              >
                {saveState === "saving" ? "Saving…" : saveState === "saved" ? "Saved" : saveState === "error" ? "Save failed" : "Editing…"}
              </span>
            )}
            <Button type="button" variant="ghost" size="icon-sm" disabled={!canUndo} onClick={undo} title="Undo">
              <Undo2 />
            </Button>
            <Button type="button" variant="ghost" size="icon-sm" disabled={!canRedo} onClick={redo} title="Redo">
              <Redo2 />
            </Button>
            <Button type="button" size="sm" onClick={() => void openExport()}>
              <Share /> <span className="hidden sm:inline">Export</span>
            </Button>
          </div>
        </header>

        {error ? (
          <div className="grid flex-1 place-items-center p-6">
            <Alert variant="destructive" className="max-w-md">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 flex-col">
            <main className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-2.5 lg:grid lg:grid-cols-[minmax(280px,21vw)_minmax(0,1fr)_minmax(300px,22vw)] lg:overflow-hidden">
              <aside
                className={cn(
                  "min-h-0 overflow-hidden rounded-2xl border bg-card lg:grid lg:grid-cols-[58px_minmax(0,1fr)]",
                  mobilePanel === "media" ? "grid" : "hidden lg:grid",
                )}
              >
                <nav
                  className="flex gap-1 border-b bg-muted/40 p-1.5 lg:flex-col lg:border-b-0 lg:border-r lg:p-2"
                  aria-label="Editor tools"
                >
                  {railItems.map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => setActiveToolPanel(item.id)}
                      title={`${item.label} tools`}
                      className={cn(
                        "grid min-h-11 flex-1 place-items-center gap-1 rounded-xl border-0 bg-transparent text-[11px] font-semibold text-muted-foreground transition-colors lg:min-h-[52px] lg:flex-none",
                        activeToolPanel === item.id ? "bg-card text-foreground shadow-xs" : "hover:bg-muted hover:text-foreground",
                      )}
                    >
                      <item.icon className="h-4 w-4" />
                      <span>{item.label}</span>
                    </button>
                  ))}
                </nav>
                <div className="grid min-h-0 overflow-hidden">
                  {activeToolPanel === "media" && (
                    <MediaBin token={token} parentID={node.parent_id || "root"} onAdd={(media) => void addMediaClip(media)} onChanged={onDone} />
                  )}
                  {activeToolPanel === "text" && (
                    <div className="grid content-start gap-3 overflow-auto p-3.5">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="text-sm font-semibold">Text</p>
                          <p className="text-xs text-muted-foreground">Add titles or labels over your video.</p>
                        </div>
                      </div>
                      <Button type="button" variant="outline" className="h-auto w-full justify-start gap-3 whitespace-normal p-3.5 text-left" onClick={() => addTextClip("text")}>
                        <Type className="h-4 w-4 shrink-0" />
                        <span className="grid gap-0.5"><strong className="text-sm font-medium">Title text</strong><small className="text-xs text-muted-foreground">Add editable overlay at playhead</small></span>
                      </Button>
                      <Button type="button" variant="outline" className="h-auto w-full justify-start gap-3 whitespace-normal p-3.5 text-left" onClick={() => addTextClip("caption")}>
                        <Captions className="h-4 w-4 shrink-0" />
                        <span className="grid gap-0.5"><strong className="text-sm font-medium">Quick caption</strong><small className="text-xs text-muted-foreground">Add subtitle-style text</small></span>
                      </Button>
                    </div>
                  )}
                  {activeToolPanel === "captions" && (
                    <div className="grid content-start gap-3 overflow-auto p-3.5">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="text-sm font-semibold">Captions</p>
                          <p className="text-xs text-muted-foreground">Import or export SRT subtitles.</p>
                        </div>
                      </div>
                      <Button type="button" variant="outline" className="h-auto w-full justify-start gap-3 whitespace-normal p-3.5 text-left" disabled={captionBusy} onClick={() => captionInput.current?.click()}>
                        <FileInput className="h-4 w-4 shrink-0" />
                        <span className="grid gap-0.5"><strong className="text-sm font-medium">Import SRT</strong><small className="text-xs text-muted-foreground">Create caption clips from a subtitle file</small></span>
                      </Button>
                      {project && (
                        <Button asChild variant="outline" className="h-auto w-full justify-start gap-3 whitespace-normal p-3.5 text-left">
                          <a href={captionExportUrl(token, project.id)}>
                            <FileText className="h-4 w-4 shrink-0" />
                            <span className="grid gap-0.5"><strong className="text-sm font-medium">Export SRT</strong><small className="text-xs text-muted-foreground">Download captions from this project</small></span>
                          </a>
                        </Button>
                      )}
                    </div>
                  )}
                </div>
              </aside>
              <div className={cn("min-h-0", mobilePanel === "preview" || mobilePanel === "timeline" ? "block" : "hidden lg:block")}>
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
              </div>
              <div className={cn("min-h-0", mobilePanel === "properties" ? "block" : "hidden lg:block")}>
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
              </div>
            </main>

            <nav
              className="flex shrink-0 gap-1 overflow-x-auto border-t bg-card px-2 py-1.5 lg:hidden"
              aria-label="Editing shortcuts"
            >
              {quickActions.map((action) => (
                <button
                  key={action.label}
                  type="button"
                  onClick={action.onClick}
                  disabled={action.disabled}
                  title={action.title}
                  className={cn(
                    "grid min-h-[52px] min-w-[60px] flex-1 place-items-center gap-0.5 rounded-xl border border-transparent px-2 text-[11px] font-semibold transition-colors",
                    action.primary
                      ? "bg-primary text-primary-foreground hover:bg-primary/90"
                      : action.active
                        ? "bg-muted text-foreground"
                        : "text-muted-foreground hover:bg-muted hover:text-foreground",
                    action.disabled && "cursor-not-allowed opacity-40",
                  )}
                >
                  <action.icon className="h-4 w-4" />
                  <span>{action.label}</span>
                </button>
              ))}
            </nav>

            <div className="block min-h-0 shrink-0">
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
          </div>
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
