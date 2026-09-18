import type { MediaJobStatus, TimelineClip } from "../../api";

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
      <aside className="editor-property-panel">
        <div className="editor-property-tabs" aria-hidden="true">
          <span className="active"><i className="fa-solid fa-sliders" /></span>
        </div>
        <div className="editor-inspector">
          <p className="editor-panel-title">Properties</p>
          <p className="editor-muted">Select a clip on the timeline.</p>
        </div>
      </aside>
    );
  }
  const duration = Math.max(0, clip.endInSource - clip.startInSource);
  return (
    <aside className="editor-property-panel">
      <div className="editor-property-tabs" aria-label="Property sections">
        <button type="button" className="active" title="Timing"><i className="fa-solid fa-sliders" /></button>
        <button type="button" title="Audio and effects"><i className="fa-solid fa-wand-magic-sparkles" /></button>
      </div>
      <div className="editor-inspector">
      <div className="editor-panel-head">
        <div>
          <p className="editor-panel-title">Properties</p>
          <p className="editor-muted truncate">{sourceName}</p>
        </div>
      </div>
      <div className="editor-section">
        <span className="editor-section-title">Timing</span>
        <label>
          Trim in
          <input
            type="number"
            min={0}
            step={0.05}
            value={clip.startInSource}
            onChange={(e) => onChange(clip.id, { startInSource: Number(e.target.value) })}
          />
        </label>
        <label>
          Trim out
          <input
            type="number"
            min={0}
            step={0.05}
            value={clip.endInSource}
            onChange={(e) => onChange(clip.id, { endInSource: Number(e.target.value) })}
          />
        </label>
        <label>
          Speed
          <input
            type="number"
            min={0.1}
            max={16}
            step={0.1}
            value={clip.speed ?? 1}
            onChange={(e) => onChange(clip.id, { speed: Number(e.target.value) })}
          />
        </label>
        <label>
          Timeline start
          <input
            type="number"
            min={0}
            step={0.05}
            value={clip.startOnTimeline}
            onChange={(e) => onChange(clip.id, { startOnTimeline: Number(e.target.value) })}
          />
        </label>
      </div>
      <div className="editor-section">
        <span className="editor-section-title">Audio & Effects</span>
        {canExtractAudio && (
          <button
            type="button"
            className="editor-create-card clip-extract-card"
            disabled={extractAudioJob?.status === "queued" || extractAudioJob?.status === "running"}
            onClick={onExtractAudio}
          >
            <i className="fa-solid fa-music" />
            <span>
              <strong>Extract audio from this clip</strong>
              <small>{extractAudioJob ? clipAudioJobLabel(extractAudioJob) : "Save the video audio as reusable .m4a"}</small>
            </span>
          </button>
        )}
        <label>
          Volume
          <input
            type="range"
            min={0}
            max={2}
            step={0.05}
            value={clip.volume ?? 1}
            onChange={(e) => onChange(clip.id, { volume: Number(e.target.value), hasEffects: Number(e.target.value) !== 1 })}
          />
        </label>
        <div className="editor-two-col">
          <label>
            Fade in
            <input
              type="number"
              min={0}
              step={0.05}
              value={clip.fade_in ?? 0}
              onChange={(e) => onChange(clip.id, { fade_in: Number(e.target.value), hasEffects: Number(e.target.value) > 0 })}
            />
          </label>
          <label>
            Fade out
            <input
              type="number"
              min={0}
              step={0.05}
              value={clip.fade_out ?? 0}
              onChange={(e) => onChange(clip.id, { fade_out: Number(e.target.value), hasEffects: Number(e.target.value) > 0 })}
            />
          </label>
        </div>
        <label>
          Transition
          <select
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
          </select>
        </label>
      </div>
      <div className="editor-stat">
        <span>Duration</span>
        <strong>{duration.toFixed(2)}s</strong>
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
