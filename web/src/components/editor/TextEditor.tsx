import type { TimelineClip, TextOverlay } from "../../api";

type Props = {
  clip: TimelineClip | null;
  kind: "text" | "caption";
  onChange: (clipID: string, patch: Partial<TimelineClip>) => void;
};

const DEFAULT_TEXT: TextOverlay = {
  content: "",
  font_size: 42,
  font_family: "Inter",
  color: "#ffffff",
  bg_color: "rgba(0,0,0,0.45)",
  x: 0.5,
  y: 0.5,
  alignment: "center",
  animation: "none",
};

export function TextEditor({ clip, kind, onChange }: Props) {
  if (!clip) {
    return (
      <aside className="editor-property-panel">
        <div className="editor-property-tabs" aria-hidden="true">
          <span className="active"><i className="fa-solid fa-font" /></span>
        </div>
        <div className="editor-inspector text-editor">
          <p className="editor-panel-title">{kind === "caption" ? "Caption" : "Text"}</p>
          <p className="editor-muted">Select a text clip.</p>
        </div>
      </aside>
    );
  }
  const text = { ...DEFAULT_TEXT, ...(clip.text ?? {}) };
  const patchText = (patch: Partial<TextOverlay>) => onChange(clip.id, { text: { ...text, ...patch } });
  return (
    <aside className="editor-property-panel">
      <div className="editor-property-tabs" aria-label="Text property sections">
        <button type="button" className="active" title="Content"><i className="fa-solid fa-font" /></button>
        <button type="button" title="Style"><i className="fa-solid fa-palette" /></button>
        <button type="button" title="Position"><i className="fa-solid fa-arrows-up-down-left-right" /></button>
      </div>
      <div className="editor-inspector text-editor">
      <div className="editor-panel-head">
        <div>
          <p className="editor-panel-title">{kind === "caption" ? "Caption" : "Text"}</p>
          <p className="editor-muted">Style and position the selected overlay.</p>
        </div>
      </div>
      <div className="editor-section">
        <span className="editor-section-title">Content</span>
        <label>
          Text
          <textarea value={text.content} rows={4} onChange={(e) => patchText({ content: e.target.value })} />
        </label>
        <div className="editor-two-col">
          <label>
            Start
            <input type="number" min={0} step={0.05} value={clip.startOnTimeline} onChange={(e) => onChange(clip.id, { startOnTimeline: Number(e.target.value) })} />
          </label>
          <label>
            Duration
            <input type="number" min={0.1} step={0.05} value={clip.endInSource - clip.startInSource} onChange={(e) => onChange(clip.id, { endInSource: clip.startInSource + Number(e.target.value) })} />
          </label>
        </div>
      </div>
      <div className="editor-section">
        <span className="editor-section-title">Style</span>
        <label>
          Font size
          <input type="range" min={12} max={200} value={text.font_size} onChange={(e) => patchText({ font_size: Number(e.target.value) })} />
        </label>
        <div className="editor-two-col">
          <label>
            Text color
            <input type="color" value={cssColorToHex(text.color, "#ffffff")} onChange={(e) => patchText({ color: e.target.value })} />
          </label>
          <label>
            Background
            <input type="color" value={cssColorToHex(text.bg_color, "#000000")} onChange={(e) => patchText({ bg_color: `${e.target.value}cc` })} />
          </label>
        </div>
        <label>
          Align
          <select value={text.alignment} onChange={(e) => patchText({ alignment: e.target.value as TextOverlay["alignment"] })}>
            <option value="center">Center</option>
            <option value="left">Left</option>
            <option value="right">Right</option>
          </select>
        </label>
      </div>
      <div className="editor-section">
        <span className="editor-section-title">Position & Motion</span>
        <div className="editor-two-col">
          <label>
            X
            <input type="range" min={0} max={1} step={0.01} value={text.x} onChange={(e) => patchText({ x: Number(e.target.value) })} />
          </label>
          <label>
            Y
            <input type="range" min={0} max={1} step={0.01} value={text.y} onChange={(e) => patchText({ y: Number(e.target.value) })} />
          </label>
        </div>
        <label>
          Animation
          <select value={text.animation} onChange={(e) => patchText({ animation: e.target.value as TextOverlay["animation"] })}>
            <option value="none">None</option>
            <option value="fade">Fade</option>
            <option value="slide-up">Slide up</option>
            <option value="typewriter">Typewriter</option>
          </select>
        </label>
      </div>
      </div>
    </aside>
  );
}

function cssColorToHex(value: string, fallback: string) {
  return /^#[0-9a-f]{6}/i.test(value) ? value.slice(0, 7) : fallback;
}
