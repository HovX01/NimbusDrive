import { useEffect, useMemo, useRef, useState } from "react";
import { Pause, Play } from "lucide-react";
import { getProxyStatus, mediaStreamUrl, proxyStreamUrl } from "../../api";
import type { Node, TimelineClip, TimelineData } from "../../api";
import { previewKind } from "../../lib/files";
import { cn } from "@/lib/utils";
import { Button } from "../ui/button";

type Props = {
  src: string;
  token: string;
  sourceNodeID: string;
  mediaByID: Record<string, Node>;
  timeline: TimelineData;
  playhead: number;
  playing: boolean;
  onPlayhead: (seconds: number) => void;
  onPlaying: (playing: boolean) => void;
  onDuration: (seconds: number) => void;
};

function clipTimelineDuration(clip: TimelineClip) {
  return Math.max(0.1, (clip.endInSource - clip.startInSource) / Math.max(0.1, clip.speed || 1));
}

function clipsInOrder(timeline: TimelineData) {
  return [...(timeline.tracks.find((track) => track.type === "video" && !track.muted)?.clips ?? [])].sort((a, b) => a.startOnTimeline - b.startOnTimeline);
}

function activeClipAtPlayhead(timeline: TimelineData, playhead: number) {
  return clipsInOrder(timeline).find((item) => {
    const duration = clipTimelineDuration(item);
    return playhead >= item.startOnTimeline && playhead <= item.startOnTimeline + duration;
  });
}

function sourceTimeForPlayhead(timeline: TimelineData, playhead: number) {
  const clips = clipsInOrder(timeline);
  const clip = activeClipAtPlayhead(timeline, playhead);
  if (!clip) return clips[0]?.startInSource ?? 0;
  return clip.startInSource + Math.max(0, playhead - clip.startOnTimeline) * Math.max(0.1, clip.speed || 1);
}

function textOverlays(timeline: TimelineData, playhead: number) {
  return timeline.tracks
    .filter((track) => !track.muted && (track.type === "text" || track.type === "caption"))
    .flatMap((track) => track.clips.map((clip) => ({ clip, type: track.type })))
    .filter(({ clip }) => {
      const end = clip.startOnTimeline + Math.max(0.1, clip.endInSource - clip.startInSource);
      return playhead >= clip.startOnTimeline && playhead <= end && clip.text?.content;
    });
}

function activeAudioClipAtPlayhead(timeline: TimelineData, playhead: number) {
  return timeline.tracks
    .filter((track) => track.type === "audio" && !track.muted)
    .flatMap((track) => track.clips)
    .find((clip) => {
      const duration = clipTimelineDuration(clip);
      return playhead >= clip.startOnTimeline && playhead <= clip.startOnTimeline + duration;
    }) ?? null;
}

export function Preview({ src, token, sourceNodeID, mediaByID, timeline, playhead, playing, onPlayhead, onPlaying, onDuration }: Props) {
  const video = useRef<HTMLVideoElement>(null);
  const audio = useRef<HTMLAudioElement>(null);
  const imageTimer = useRef<number | null>(null);
  const imageTimerLast = useRef(0);
  const playheadRef = useRef(playhead);
  const timelineRef = useRef(timeline);
  const [proxyStatus, setProxyStatus] = useState<"none" | "generating" | "ready" | "error">("none");
  const [visualAspect, setVisualAspect] = useState(16 / 9);
  const desiredSourceTime = useMemo(() => sourceTimeForPlayhead(timeline, playhead), [timeline, playhead]);
  const activeClip = useMemo(() => activeClipAtPlayhead(timeline, playhead), [timeline, playhead]);
  const activeAudioClip = useMemo(() => activeAudioClipAtPlayhead(timeline, playhead), [timeline, playhead]);
  const activeOverlays = useMemo(() => textOverlays(timeline, playhead), [timeline, playhead]);
  const activeSourceID = activeClip?.sourceNodeId || sourceNodeID;
  const activeNode = mediaByID[activeSourceID] ?? mediaByID[sourceNodeID];
  const activeKind = activeNode ? previewKind(activeNode.name, activeNode.mime_type, false) : "video";
  const sourceSrc = activeSourceID === sourceNodeID ? src : mediaStreamUrl(token, activeSourceID);
  const videoSrc = proxyStatus === "ready" && activeSourceID === sourceNodeID ? proxyStreamUrl(token, sourceNodeID) : sourceSrc;
  const audioSrc = activeAudioClip?.sourceNodeId ? mediaStreamUrl(token, activeAudioClip.sourceNodeId) : "";
  const desiredAudioTime = activeAudioClip
    ? activeAudioClip.startInSource + Math.max(0, playhead - activeAudioClip.startOnTimeline) * Math.max(0.1, activeAudioClip.speed || 1)
    : 0;
  const canvasAspect = Number.isFinite(visualAspect) && visualAspect > 0 ? visualAspect : 16 / 9;

  function togglePlayback() {
    if (playing) {
      onPlaying(false);
      return;
    }
    if (timeline.duration > 0 && playhead >= timeline.duration - 0.05) {
      playheadRef.current = 0;
      onPlayhead(0);
    }
    onPlaying(true);
  }

  useEffect(() => {
    playheadRef.current = playhead;
  }, [playhead]);

  useEffect(() => {
    timelineRef.current = timeline;
  }, [timeline]);

  useEffect(() => {
    let cancelled = false;
    let timer: number | null = null;
    function poll() {
      getProxyStatus(token, sourceNodeID)
        .then((status) => {
          if (cancelled) return;
          setProxyStatus(status.status);
          if (status.status === "generating") timer = window.setTimeout(poll, 2500);
        })
        .catch(() => {
          if (!cancelled) setProxyStatus("none");
        });
    }
    poll();
    return () => {
      cancelled = true;
      if (timer !== null) window.clearTimeout(timer);
    };
  }, [sourceNodeID, token]);

  useEffect(() => {
    const el = video.current;
    if (!el) return;
    if (Math.abs(el.currentTime - desiredSourceTime) > 0.25) {
      el.currentTime = desiredSourceTime;
    }
  }, [desiredSourceTime]);

  useEffect(() => {
    const el = video.current;
    if (!el) return;
    el.playbackRate = Math.max(0.1, activeClip?.speed || 1);
  }, [activeClip]);

  useEffect(() => {
    const el = video.current;
    if (!el) return;
    if (activeKind !== "video") {
      el.pause();
      return;
    }
    if (playing) {
      void el.play().catch(() => onPlaying(false));
    } else {
      el.pause();
    }
  }, [activeKind, playing, onPlaying]);

  useEffect(() => {
    setVisualAspect(16 / 9);
  }, [activeSourceID]);

  useEffect(() => {
    const el = audio.current;
    if (!el) return;
    if (!activeAudioClip) {
      el.pause();
      return;
    }
    const threshold = playing ? 1.25 : 0.15;
    if (Math.abs(el.currentTime - desiredAudioTime) > threshold) {
      el.currentTime = desiredAudioTime;
    }
    el.playbackRate = Math.max(0.1, activeAudioClip.speed || 1);
    el.volume = Math.max(0, Math.min(1, activeAudioClip.volume ?? 1));
  }, [activeAudioClip, desiredAudioTime, playing]);

  useEffect(() => {
    const el = audio.current;
    if (!el) return;
    if (playing && activeAudioClip) {
      void el.play().catch(() => undefined);
    } else {
      el.pause();
    }
  }, [activeAudioClip, playing]);

  useEffect(() => {
    if (imageTimer.current !== null) {
      window.cancelAnimationFrame(imageTimer.current);
      imageTimer.current = null;
    }
    if (!playing || activeKind === "video") return;
    imageTimerLast.current = performance.now();
    const tick = (now: number) => {
      const delta = (now - imageTimerLast.current) / 1000;
      imageTimerLast.current = now;
      const audioEl = audio.current;
      const currentAudioClip = activeAudioClipAtPlayhead(timelineRef.current, playheadRef.current);
      const next = audioEl && currentAudioClip && !audioEl.paused
        ? currentAudioClip.startOnTimeline + (audioEl.currentTime - currentAudioClip.startInSource) / Math.max(0.1, currentAudioClip.speed || 1)
        : playheadRef.current + delta;
      const clamped = Math.min(timelineRef.current.duration, Math.max(0, next));
      playheadRef.current = clamped;
      onPlayhead(clamped);
      if (clamped >= timelineRef.current.duration) {
        onPlaying(false);
        return;
      }
      imageTimer.current = window.requestAnimationFrame(tick);
    };
    imageTimer.current = window.requestAnimationFrame(tick);
    return () => {
      if (imageTimer.current !== null) window.cancelAnimationFrame(imageTimer.current);
      imageTimer.current = null;
    };
  }, [activeKind, onPlayhead, onPlaying, playing]);

  return (
    <section className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)_52px] gap-3 rounded-2xl border bg-card p-3.5">
      <div className="flex w-full items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="text-sm font-semibold">Preview</p>
          <span className="block truncate text-xs text-muted-foreground">{activeNode?.name || "Timeline output"}</span>
        </div>
        <span
          className={cn(
            "rounded-full border bg-muted px-2.5 py-1 text-xs font-semibold text-muted-foreground",
            proxyStatus === "ready" && "border-emerald-500/30 bg-emerald-500/10 text-emerald-600",
            proxyStatus === "generating" && "border-amber-500/30 bg-amber-500/10 text-amber-600",
          )}
        >
          {activeKind === "image" ? "Image" : proxyStatus === "ready" ? "Proxy" : proxyStatus === "generating" ? "Proxying" : "Original"}
        </span>
      </div>
      <div className="relative grid min-h-0 w-full place-items-center overflow-hidden rounded-2xl border bg-muted/30">
        <div
          className="relative grid max-h-full max-w-full place-items-center overflow-hidden rounded-lg bg-black shadow-lg"
          style={{
            aspectRatio: canvasAspect,
            width: canvasAspect >= 1 ? "100%" : undefined,
            height: canvasAspect < 1 ? "100%" : undefined,
          }}
        >
          {activeKind === "image" ? (
            <img
              className="h-full w-full bg-black object-contain"
              src={videoSrc}
              alt=""
              onLoad={(e) => {
                const img = e.currentTarget;
                if (img.naturalWidth && img.naturalHeight) setVisualAspect(img.naturalWidth / img.naturalHeight);
              }}
            />
          ) : (
            <video
              ref={video}
              className="h-full w-full bg-black object-contain"
              src={videoSrc}
              playsInline
              preload="auto"
              onLoadedMetadata={(e) => {
                const currentVideo = e.currentTarget;
                onDuration(currentVideo.duration || 0);
                if (currentVideo.videoWidth && currentVideo.videoHeight) {
                  setVisualAspect(currentVideo.videoWidth / currentVideo.videoHeight);
                }
              }}
              onPlay={() => onPlaying(true)}
              onPause={() => onPlaying(false)}
              onTimeUpdate={(e) => {
                if (!playing) return;
                const sourceNow = e.currentTarget.currentTime;
                const clip = activeClipAtPlayhead(timeline, playhead);
                if (!clip) {
                  onPlaying(false);
                  return;
                }
                if (sourceNow >= clip.endInSource) {
                  const clips = clipsInOrder(timeline);
                  const nextClip = clips.find((item) => item.startOnTimeline > clip.startOnTimeline);
                  if (!nextClip) {
                    onPlayhead(timeline.duration);
                    onPlaying(false);
                    return;
                  }
                  e.currentTarget.currentTime = nextClip.startInSource;
                  onPlayhead(nextClip.startOnTimeline);
                  return;
                }
                if (sourceNow < clip.startInSource) {
                  e.currentTarget.currentTime = clip.startInSource;
                  return;
                }
                const nextPlayhead = clip.startOnTimeline + (sourceNow - clip.startInSource) / Math.max(0.1, clip.speed || 1);
                onPlayhead(Math.min(timeline.duration, nextPlayhead));
                if (nextPlayhead >= timeline.duration) onPlaying(false);
              }}
            />
          )}
          {activeOverlays.map(({ clip, type }) => (
            <div
              key={clip.id}
              className={cn(
                "pointer-events-none absolute z-[3] max-w-[78%] whitespace-pre-wrap rounded-lg px-[0.45em] py-[0.2em] leading-tight [transform:translate(-50%,-50%)] [text-shadow:0_2px_10px_rgba(0,0,0,0.45)]",
                type === "caption" && "max-w-[86%]",
              )}
              style={{
                left: `${Math.max(0, Math.min(1, clip.text?.x ?? 0.5)) * 100}%`,
                top: `${Math.max(0, Math.min(1, clip.text?.y ?? 0.5)) * 100}%`,
                color: clip.text?.color || "#fff",
                background: clip.text?.bg_color || "transparent",
                fontSize: `${clip.text?.font_size || 42}px`,
                textAlign: clip.text?.alignment || "center",
              }}
            >
              {clip.text?.content}
            </div>
          ))}
        </div>
        <audio ref={audio} src={audioSrc || undefined} preload="auto" />
      </div>
      <div className="flex w-full items-center justify-center gap-3 border-t pt-3 font-mono text-xs text-muted-foreground">
        <Button type="button" variant="ghost" size="icon-sm" onClick={togglePlayback} title={playing ? "Pause" : "Play"}>
          {playing ? <Pause /> : <Play />}
        </Button>
        <span>{playhead.toFixed(2)}s / {Math.max(timeline.duration, 0).toFixed(2)}s</span>
      </div>
    </section>
  );
}
