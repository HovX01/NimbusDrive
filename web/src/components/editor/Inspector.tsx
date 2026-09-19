import { Music, SlidersHorizontal, WandSparkles } from "lucide-react";
import type { MediaJobStatus, TimelineClip } from "../../api";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Select } from "../ui/select";
import { Slider } from "../ui/slider";

type Props = {
  clip: TimelineClip | null;
  sourceName: string;
  canExtractAudio?: boolean;
  extractAudioJob?: MediaJobStatus | null;
  onExtractAudio?: () => void;
  onChange: (clipID: string, patch: Partial<TimelineClip>) => void;
};

export function Inspector({ clip, sourceName, canExtractAudio = false, extractAudioJob, onExtractAudio, onChange }: Props) {
  if (!clip) {
    return (
      <aside className="grid min-h-0 grid-cols-[46px_minmax(0,1fr)] overflow-hidden rounded-2xl border bg-card">
        <div className="flex flex-col gap-1.5 border-r bg-muted/40 p-1.5" aria-hidden="true">
          <span className="grid h-[34px] w-[34px] place-items-center rounded-lg bg-card text-foreground shadow-xs">
            <SlidersHorizontal className="h-4 w-4" />
          </span>
        </div>
        <div className="grid content-start gap-3.5 overflow-auto p-3.5">
          <p className="text-sm font-semibold">Properties</p>
          <p className="text-xs text-muted-foreground">Select a clip on the timeline.</p>
        </div>
      </aside>
    );
  }
  const duration = Math.max(0, clip.endInSource - clip.startInSource);
  return (
    <aside className="grid min-h-0 grid-cols-[46px_minmax(0,1fr)] overflow-hidden rounded-2xl border bg-card">
      <div className="flex flex-col gap-1.5 border-r bg-muted/40 p-1.5" aria-label="Property sections">
        <button type="button" className="grid h-[34px] w-[34px] place-items-center rounded-lg bg-card text-foreground shadow-xs" title="Timing"><SlidersHorizontal className="h-4 w-4" /></button>
        <button type="button" className="grid h-[34px] w-[34px] place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground" title="Audio and effects"><WandSparkles className="h-4 w-4" /></button>
      </div>
      <div className="grid min-h-0 content-start gap-3.5 overflow-auto p-3.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-semibold">Properties</p>
          <p className="truncate text-xs text-muted-foreground">{sourceName}</p>
        </div>
      </div>
      <div className="grid gap-3 rounded-xl border bg-muted/30 p-3.5">
        <span className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Timing</span>
        <Label className="grid gap-1.5">
          Trim in
          <Input
            type="number"
            min={0}
            step={0.05}
            value={clip.startInSource}
            onChange={(e) => onChange(clip.id, { startInSource: Number(e.target.value) })}
          />
        </Label>
        <Label className="grid gap-1.5">
          Trim out
          <Input
            type="number"
            min={0}
            step={0.05}
            value={clip.endInSource}
            onChange={(e) => onChange(clip.id, { endInSource: Number(e.target.value) })}
          />
        </Label>
        <Label className="grid gap-1.5">
          Speed
          <Input
            type="number"
            min={0.1}
            max={16}
            step={0.1}
            value={clip.speed ?? 1}
            onChange={(e) => onChange(clip.id, { speed: Number(e.target.value) })}
          />
        </Label>
        <Label className="grid gap-1.5">
          Timeline start
          <Input
            type="number"
            min={0}
            step={0.05}
            value={clip.startOnTimeline}
            onChange={(e) => onChange(clip.id, { startOnTimeline: Number(e.target.value) })}
          />
        </Label>
      </div>
      <div className="grid gap-3 rounded-xl border bg-muted/30 p-3.5">
        <span className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Audio & Effects</span>
        {canExtractAudio && (
          <Button
            type="button"
            variant="outline"
            className="h-auto w-full justify-start gap-3 whitespace-normal p-3 text-left"
            disabled={extractAudioJob?.status === "queued" || extractAudioJob?.status === "running"}
            onClick={onExtractAudio}
          >
            <Music className="h-4 w-4 shrink-0" />
            <span className="grid gap-0.5">
              <strong className="text-sm font-medium">Extract audio from this clip</strong>
              <small className="text-xs text-muted-foreground">{extractAudioJob ? clipAudioJobLabel(extractAudioJob) : "Save the video audio as reusable .m4a"}</small>
            </span>
          </Button>
        )}
        <div className="grid gap-1.5">
          <Label>Volume</Label>
          <Slider
            min={0}
            max={2}
            step={0.05}
            value={clip.volume ?? 1}
            onChange={(e) => onChange(clip.id, { volume: Number(e.target.value), hasEffects: Number(e.target.value) !== 1 })}
          />
        </div>
        <div className="grid grid-cols-2 gap-2.5">
          <Label className="grid gap-1.5">
            Fade in
            <Input
              type="number"
              min={0}
              step={0.05}
              value={clip.fade_in ?? 0}
              onChange={(e) => onChange(clip.id, { fade_in: Number(e.target.value), hasEffects: Number(e.target.value) > 0 })}
            />
          </Label>
          <Label className="grid gap-1.5">
            Fade out
            <Input
              type="number"
              min={0}
              step={0.05}
              value={clip.fade_out ?? 0}
              onChange={(e) => onChange(clip.id, { fade_out: Number(e.target.value), hasEffects: Number(e.target.value) > 0 })}
            />
          </Label>
        </div>
        <Label className="grid gap-1.5">
          Transition
          <Select
            value={clip.transition?.type ?? ""}
            onChange={(e) => onChange(clip.id, {
              transition: e.target.value ? { type: e.target.value as NonNullable<TimelineClip["transition"]>["type"], duration: clip.transition?.duration ?? 0.5 } : undefined,
              hasEffects: !!e.target.value,
            })}
          >
            <option value="">None</option>
            <option value="crossfade">Crossfade</option>
            <option value="fade-black">Fade to black</option>
            <option value="wipe-left">Wipe left</option>
          </Select>
        </Label>
      </div>
      <div className="flex items-center justify-between gap-3 rounded-lg border bg-muted/40 p-2.5 text-sm text-muted-foreground">
        <span>Duration</span>
        <strong className="font-semibold text-foreground">{duration.toFixed(2)}s</strong>
      </div>
      </div>
    </aside>
  );
}

function clipAudioJobLabel(job: MediaJobStatus) {
  if (job.status === "done") return `Saved ${job.node?.name || "audio file"}`;
  if (job.status === "error") return job.message || "Extract failed";
  return `${job.message || job.phase} ${Math.round(job.progress)}%`;
}
