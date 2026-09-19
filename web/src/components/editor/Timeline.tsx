import { useMemo, useState, type PointerEvent } from "react";
import {
  AudioWaveform,
  Captions,
  Expand,
  Film,
  Scissors,
  Trash2,
  Type,
  Volume2,
  VolumeX,
  type LucideIcon,
} from "lucide-react";
import type { TimelineClip, TimelineData } from "../../api";
import { cn } from "@/lib/utils";
import { Button } from "../ui/button";
import { Slider } from "../ui/slider";

type Props = {
  timeline: TimelineData;
  selectedClipID: string | null;
  playhead: number;
  onSelectClip: (id: string | null) => void;
  onPlayhead: (seconds: number) => void;
  onChange: (timeline: TimelineData) => void;
  onToggleTrackMute: (trackID: string) => void;
  onSplit: () => void;
  onDelete: () => void;
};

type DragMode = "move" | "trim-start" | "trim-end";
type TimelinePointerEvent = PointerEvent<HTMLElement>;

const MIN_CLIP = 0.1;
const TRACK_HEADER_WIDTH = 132;
const TRACK_PADDING_LEFT = 24;
const LANE_OFFSET = TRACK_HEADER_WIDTH + TRACK_PADDING_LEFT;

const CLIP_GRADIENT: Record<string, string> = {
  video: "linear-gradient(135deg, #43a0ff, #6ed7a7)",
  audio: "linear-gradient(135deg, #30d158, #9be15d)",
  text: "linear-gradient(135deg, #bf5af2, #ff9f0a)",
  caption: "linear-gradient(135deg, #8e8eff, #64d2ff)",
};

function clipDuration(clip: TimelineClip) {
  return Math.max(MIN_CLIP, (clip.endInSource - clip.startInSource) / Math.max(0.1, clip.speed || 1));
}

export function Timeline({ timeline, selectedClipID, playhead, onSelectClip, onPlayhead, onChange, onToggleTrackMute, onSplit, onDelete }: Props) {
  const [zoom, setZoom] = useState(72);
  const [drag, setDrag] = useState<{ id: string; mode: DragMode; originX: number; origin: TimelineClip } | null>(null);
  const allClips = timeline.tracks.flatMap((track) => track.clips);
  const duration = Math.max(5, timeline.duration || allClips.reduce((max, clip) => Math.max(max, clip.startOnTimeline + clipDuration(clip)), 0));
  const width = Math.max(720, LANE_OFFSET + duration * zoom + 80);
  const ticks = useMemo(() => {
    const step = zoom > 90 ? 1 : zoom > 42 ? 2 : 5;
    return Array.from({ length: Math.ceil(duration / step) + 1 }, (_, i) => i * step);
  }, [duration, zoom]);

  function patchClip(id: string, patch: Partial<TimelineClip>) {
    onChange({
      ...timeline,
      tracks: timeline.tracks.map((track) => ({
        ...track,
        clips: track.clips.map((clip) => (clip.id === id ? { ...clip, ...patch } : clip)),
      })),
    });
  }

  return (
    <section className="grid grid-rows-[auto_minmax(0,1fr)] border-t bg-card">
      <div className="flex items-center justify-between gap-4 border-b px-3.5 py-2.5">
        <div className="grid gap-0.5">
          <p className="text-sm font-semibold">Timeline</p>
          <span className="text-xs text-muted-foreground">Trim edges, drag clips, split at the playhead.</span>
        </div>
        <div className="flex items-center gap-2">
          <Button type="button" variant="secondary" size="sm" onClick={onSplit} title="Split selected clip (S)">
            <Scissors />
            <span className="hidden sm:inline">Split</span>
            <kbd className="rounded border bg-muted px-1 font-mono text-[10px] text-muted-foreground">S</kbd>
          </Button>
          <Button type="button" variant="destructive" size="sm" onClick={onDelete} title="Delete clip">
            <Trash2 />
            <span className="hidden sm:inline">Delete</span>
          </Button>
          <Button type="button" variant="ghost" size="icon-sm" onClick={() => setZoom(72)} title="Reset zoom">
            <Expand />
          </Button>
          <label className="hidden items-center gap-2 text-xs text-muted-foreground sm:flex">
            Zoom
            <Slider min={28} max={140} value={zoom} onChange={(e) => setZoom(Number(e.target.value))} className="w-32" />
          </label>
        </div>
      </div>
      <div
        className="min-w-0 overflow-auto"
        onPointerMove={(e) => {
          if (!drag) return;
          const delta = (e.clientX - drag.originX) / zoom;
          if (drag.mode === "move") {
            patchClip(drag.id, { startOnTimeline: Math.max(0, drag.origin.startOnTimeline + delta) });
          } else if (drag.mode === "trim-start") {
            const speed = Math.max(0.1, drag.origin.speed || 1);
            patchClip(drag.id, {
              startInSource: Math.min(drag.origin.endInSource - MIN_CLIP, Math.max(0, drag.origin.startInSource + delta * speed)),
            });
          } else {
            const speed = Math.max(0.1, drag.origin.speed || 1);
            patchClip(drag.id, { endInSource: Math.max(drag.origin.startInSource + MIN_CLIP, drag.origin.endInSource + delta * speed) });
          }
        }}
        onPointerUp={(e) => {
          if (e.currentTarget.hasPointerCapture(e.pointerId)) {
            e.currentTarget.releasePointerCapture(e.pointerId);
          }
          setDrag(null);
        }}
        onPointerCancel={() => setDrag(null)}
      >
        <div
          className="relative min-h-[176px]"
          style={{ width }}
          onClick={(e) => {
            if (e.target !== e.currentTarget) return;
            const rect = e.currentTarget.getBoundingClientRect();
            onSelectClip(null);
            onPlayhead(Math.max(0, Math.min(duration, (e.clientX - rect.left - LANE_OFFSET) / zoom)));
          }}
        >
          <div className="relative h-[34px] border-b">
            {ticks.map((tick) => (
              <span
                key={tick}
                className="absolute top-2.5 font-mono text-xs text-muted-foreground before:absolute before:left-0 before:top-5 before:h-3 before:w-px before:bg-border"
                style={{ left: LANE_OFFSET + tick * zoom }}
              >
                {tick}s
              </span>
            ))}
          </div>
          <div className="relative grid gap-2.5 px-6 py-4 pb-5">
            {timeline.tracks.map((track, index) => {
              const TrackIcon = trackIcon(track.type);
              const MuteIcon = track.muted ? VolumeX : Volume2;
              return (
              <div
                key={track.id}
                className={cn(
                  "relative grid min-h-[72px] grid-cols-[132px_minmax(0,1fr)] rounded-xl border bg-muted/30",
                  track.muted && "opacity-50",
                )}
              >
                <div className="flex items-center gap-2 border-r px-3 text-xs font-semibold capitalize text-muted-foreground">
                  <TrackIcon className="h-4 w-4" />
                  <span>{trackLabel(track.type) || `Track ${index + 1}`}</span>
                  <Button type="button" variant="ghost" size="icon-sm" className="ml-auto" onClick={() => onToggleTrackMute(track.id)} title={track.muted ? "Unmute track" : "Mute track"}>
                    <MuteIcon />
                  </Button>
                </div>
                <div
                  className="relative min-h-[72px]"
                  onClick={(e) => {
                    if (e.target !== e.currentTarget) return;
                    const rect = e.currentTarget.getBoundingClientRect();
                    onSelectClip(null);
                    onPlayhead(Math.max(0, Math.min(duration, (e.clientX - rect.left) / zoom)));
                  }}
                >
                  {track.clips.length === 0 && (
                    <span className="pointer-events-none absolute left-4 top-1/2 -translate-y-1/2 text-xs text-muted-foreground/60">Add media here</span>
                  )}
                  {track.clips.map((clip) => {
                    const left = clip.startOnTimeline * zoom;
                    const clipWidth = Math.max(34, (track.type === "text" || track.type === "caption" ? Math.max(0.1, clip.endInSource - clip.startInSource) : clipDuration(clip)) * zoom);
                    return (
                      <button
                        key={clip.id}
                        type="button"
                        className={cn(
                          "absolute top-3.5 flex h-11 cursor-grab select-none items-center justify-center gap-1.5 overflow-hidden rounded-lg border border-white/25 px-3 font-bold text-[#061019]",
                          selectedClipID === clip.id && "ring-2 ring-white/50 ring-offset-1",
                        )}
                        style={{ left, width: clipWidth, background: CLIP_GRADIENT[track.type] ?? CLIP_GRADIENT.video }}
                        onClick={(e) => {
                          e.stopPropagation();
                          onSelectClip(clip.id);
                          onPlayhead(clip.startOnTimeline);
                        }}
                        onPointerDown={(e: TimelinePointerEvent) => {
                          onSelectClip(clip.id);
                          const scroll = e.currentTarget.closest(".timeline-scroll") as HTMLElement | null;
                          scroll?.setPointerCapture(e.pointerId);
                          setDrag({ id: clip.id, mode: "move", originX: e.clientX, origin: clip });
                        }}
                      >
                        <span
                          className="absolute inset-y-0 left-0 w-3 cursor-ew-resize bg-white/40"
                          onPointerDown={(e: TimelinePointerEvent) => {
                            e.stopPropagation();
                            const scroll = e.currentTarget.closest(".timeline-scroll") as HTMLElement | null;
                            scroll?.setPointerCapture(e.pointerId);
                            setDrag({ id: clip.id, mode: "trim-start", originX: e.clientX, origin: clip });
                          }}
                        />
                        <span className="truncate">{clip.text?.content || (track.type === "caption" ? "Caption" : track.type === "text" ? "Text" : "Clip")}</span>
                        {clip.speed && clip.speed !== 1 && <small className="text-[11px] opacity-70">{clip.speed}x</small>}
                        {clip.transition && <span className="absolute right-4 h-3 w-3 rotate-45 bg-white/80" title={clip.transition.type} />}
                        <span
                          className="absolute inset-y-0 right-0 w-3 cursor-ew-resize bg-white/40"
                          onPointerDown={(e: TimelinePointerEvent) => {
                            e.stopPropagation();
                            const scroll = e.currentTarget.closest(".timeline-scroll") as HTMLElement | null;
                            scroll?.setPointerCapture(e.pointerId);
                            setDrag({ id: clip.id, mode: "trim-end", originX: e.clientX, origin: clip });
                          }}
                        />
                      </button>
                    );
                  })}
                </div>
              </div>
              );
            })}
            <span className="pointer-events-none absolute -bottom-2.5 -top-2.5 w-0.5 bg-destructive" style={{ left: LANE_OFFSET + playhead * zoom }}>
              <span className="absolute left-1/2 top-[-6px] h-3 w-3 -translate-x-1/2 rounded-full bg-destructive" />
            </span>
          </div>
        </div>
      </div>
    </section>
  );
}

function trackLabel(type: string) {
  switch (type) {
    case "audio":
      return "Audio";
    case "text":
      return "Text";
    case "caption":
      return "Captions";
    case "video":
      return "Video";
    default:
      return type;
  }
}

function trackIcon(type: string): LucideIcon {
  switch (type) {
    case "audio":
      return AudioWaveform;
    case "text":
      return Type;
    case "caption":
      return Captions;
    default:
      return Film;
  }
}
