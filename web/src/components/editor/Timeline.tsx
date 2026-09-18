import { useMemo, useState, type PointerEvent } from "react";
import type { TimelineClip, TimelineData } from "../../api";

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
    <section className="editor-timeline">
      <div className="editor-timeline-head">
        <div className="editor-timeline-title">
          <p className="editor-panel-title">Timeline</p>
          <span className="editor-muted">Trim edges, drag clips, split at the playhead.</span>
        </div>
        <div className="editor-timeline-actions">
          <button type="button" className="editor-action-pill" onClick={onSplit} title="Split selected clip (S)">
            <i className="fa-solid fa-scissors" />
            <span>Split</span>
            <kbd>S</kbd>
          </button>
          <button type="button" className="editor-action-pill danger" onClick={onDelete} title="Delete clip">
            <i className="fa-solid fa-trash" />
            <span>Delete</span>
          </button>
          <button type="button" className="editor-tool-btn" onClick={() => setZoom(72)} title="Reset zoom">
            <i className="fa-solid fa-expand" />
          </button>
          <label className="editor-zoom">
            Zoom
            <input type="range" min={28} max={140} value={zoom} onChange={(e) => setZoom(Number(e.target.value))} />
          </label>
        </div>
      </div>
      <div
        className="timeline-scroll"
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
          className="timeline-canvas"
          style={{ width }}
          onClick={(e) => {
            if (e.target !== e.currentTarget) return;
            const rect = e.currentTarget.getBoundingClientRect();
            onSelectClip(null);
            onPlayhead(Math.max(0, Math.min(duration, (e.clientX - rect.left - LANE_OFFSET) / zoom)));
          }}
        >
          <div className="timeline-ruler">
            {ticks.map((tick) => (
              <span key={tick} style={{ left: LANE_OFFSET + tick * zoom }}>{tick}s</span>
            ))}
          </div>
          <div className="timeline-tracks">
            {timeline.tracks.map((track, index) => (
              <div key={track.id} className={`timeline-track ${track.type} ${track.muted ? "muted" : ""}`}>
                <div className="timeline-track-header">
                  <i className={trackIcon(track.type)} />
                  <span>{trackLabel(track.type) || `Track ${index + 1}`}</span>
                  <button type="button" onClick={() => onToggleTrackMute(track.id)} title={track.muted ? "Unmute track" : "Mute track"}>
                    <i className={`fa-solid ${track.muted ? "fa-volume-xmark" : "fa-volume-high"}`} />
                  </button>
                </div>
                <div
                  className="timeline-track-lane"
                  onClick={(e) => {
                    if (e.target !== e.currentTarget) return;
                    const rect = e.currentTarget.getBoundingClientRect();
                    onSelectClip(null);
                    onPlayhead(Math.max(0, Math.min(duration, (e.clientX - rect.left) / zoom)));
                  }}
                >
                  {track.clips.length === 0 && <span className="timeline-empty">Add media here</span>}
                  {track.clips.map((clip) => {
                    const left = clip.startOnTimeline * zoom;
                    const clipWidth = Math.max(34, (track.type === "text" || track.type === "caption" ? Math.max(0.1, clip.endInSource - clip.startInSource) : clipDuration(clip)) * zoom);
                    return (
                      <button
                        key={clip.id}
                        type="button"
                        className={`timeline-clip ${track.type} ${selectedClipID === clip.id ? "selected" : ""}`}
                        style={{ left, width: clipWidth }}
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
                          className="timeline-handle start"
                          onPointerDown={(e: TimelinePointerEvent) => {
                            e.stopPropagation();
                            const scroll = e.currentTarget.closest(".timeline-scroll") as HTMLElement | null;
                            scroll?.setPointerCapture(e.pointerId);
                            setDrag({ id: clip.id, mode: "trim-start", originX: e.clientX, origin: clip });
                          }}
                        />
                        <span className="truncate">{clip.text?.content || (track.type === "caption" ? "Caption" : track.type === "text" ? "Text" : "Clip")}</span>
                        {clip.speed && clip.speed !== 1 && <small>{clip.speed}x</small>}
                        {clip.transition && <span className="transition-marker" title={clip.transition.type} />}
                        <span
                          className="timeline-handle end"
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
            ))}
            <span className="timeline-playhead" style={{ left: LANE_OFFSET + playhead * zoom }} />
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

function trackIcon(type: string) {
  switch (type) {
    case "audio":
      return "fa-solid fa-wave-square";
    case "text":
      return "fa-solid fa-font";
    case "caption":
      return "fa-solid fa-closed-captioning";
    default:
      return "fa-solid fa-film";
  }
}
