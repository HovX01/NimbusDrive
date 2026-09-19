import { Move, Palette, Type } from "lucide-react";
import type { TimelineClip, TextOverlay } from "../../api";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Select } from "../ui/select";
import { Slider } from "../ui/slider";
import { Textarea } from "../ui/textarea";

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
      <aside className="grid min-h-0 grid-cols-[46px_minmax(0,1fr)] overflow-hidden rounded-2xl border bg-card">
        <div className="flex flex-col gap-1.5 border-r bg-muted/40 p-1.5" aria-hidden="true">
          <span className="grid h-[34px] w-[34px] place-items-center rounded-lg bg-card text-foreground shadow-xs">
            <Type className="h-4 w-4" />
          </span>
        </div>
        <div className="grid content-start gap-3.5 overflow-auto p-3.5">
          <p className="text-sm font-semibold">{kind === "caption" ? "Caption" : "Text"}</p>
          <p className="text-xs text-muted-foreground">Select a text clip.</p>
        </div>
      </aside>
    );
  }
  const text = { ...DEFAULT_TEXT, ...(clip.text ?? {}) };
  const patchText = (patch: Partial<TextOverlay>) => onChange(clip.id, { text: { ...text, ...patch } });
  return (
    <aside className="grid min-h-0 grid-cols-[46px_minmax(0,1fr)] overflow-hidden rounded-2xl border bg-card">
      <div className="flex flex-col gap-1.5 border-r bg-muted/40 p-1.5" aria-label="Text property sections">
        <button type="button" className="grid h-[34px] w-[34px] place-items-center rounded-lg bg-card text-foreground shadow-xs" title="Content"><Type className="h-4 w-4" /></button>
        <button type="button" className="grid h-[34px] w-[34px] place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground" title="Style"><Palette className="h-4 w-4" /></button>
        <button type="button" className="grid h-[34px] w-[34px] place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground" title="Position"><Move className="h-4 w-4" /></button>
      </div>
      <div className="grid min-h-0 content-start gap-3.5 overflow-auto p-3.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-semibold">{kind === "caption" ? "Caption" : "Text"}</p>
          <p className="text-xs text-muted-foreground">Style and position the selected overlay.</p>
        </div>
      </div>
      <div className="grid gap-3 rounded-xl border bg-muted/30 p-3.5">
        <span className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Content</span>
        <Label className="grid gap-1.5">
          Text
          <Textarea value={text.content} rows={4} onChange={(e) => patchText({ content: e.target.value })} />
        </Label>
        <div className="grid grid-cols-2 gap-2.5">
          <Label className="grid gap-1.5">
            Start
            <Input type="number" min={0} step={0.05} value={clip.startOnTimeline} onChange={(e) => onChange(clip.id, { startOnTimeline: Number(e.target.value) })} />
          </Label>
          <Label className="grid gap-1.5">
            Duration
            <Input type="number" min={0.1} step={0.05} value={clip.endInSource - clip.startInSource} onChange={(e) => onChange(clip.id, { endInSource: clip.startInSource + Number(e.target.value) })} />
          </Label>
        </div>
      </div>
      <div className="grid gap-3 rounded-xl border bg-muted/30 p-3.5">
        <span className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Style</span>
        <div className="grid gap-1.5">
          <Label>Font size</Label>
          <Slider min={12} max={200} value={text.font_size} onChange={(e) => patchText({ font_size: Number(e.target.value) })} />
        </div>
        <div className="grid grid-cols-2 gap-2.5">
          <Label className="grid gap-1.5">
            Text color
            <Input type="color" className="h-10 cursor-pointer p-1" value={cssColorToHex(text.color, "#ffffff")} onChange={(e) => patchText({ color: e.target.value })} />
          </Label>
          <Label className="grid gap-1.5">
            Background
            <Input type="color" className="h-10 cursor-pointer p-1" value={cssColorToHex(text.bg_color, "#000000")} onChange={(e) => patchText({ bg_color: `${e.target.value}cc` })} />
          </Label>
        </div>
        <Label className="grid gap-1.5">
          Align
          <Select value={text.alignment} onChange={(e) => patchText({ alignment: e.target.value as TextOverlay["alignment"] })}>
            <option value="center">Center</option>
            <option value="left">Left</option>
            <option value="right">Right</option>
          </Select>
        </Label>
      </div>
      <div className="grid gap-3 rounded-xl border bg-muted/30 p-3.5">
        <span className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Position &amp; Motion</span>
        <div className="grid grid-cols-2 gap-2.5">
          <div className="grid gap-1.5">
            <Label>X</Label>
            <Slider min={0} max={1} step={0.01} value={text.x} onChange={(e) => patchText({ x: Number(e.target.value) })} />
          </div>
          <div className="grid gap-1.5">
            <Label>Y</Label>
            <Slider min={0} max={1} step={0.01} value={text.y} onChange={(e) => patchText({ y: Number(e.target.value) })} />
          </div>
        </div>
        <Label className="grid gap-1.5">
          Animation
          <Select value={text.animation} onChange={(e) => patchText({ animation: e.target.value as TextOverlay["animation"] })}>
            <option value="none">None</option>
            <option value="fade">Fade</option>
            <option value="slide-up">Slide up</option>
            <option value="typewriter">Typewriter</option>
          </Select>
        </Label>
      </div>
      </div>
    </aside>
  );
}

function cssColorToHex(value: string, fallback: string) {
  return /^#[0-9a-f]{6}/i.test(value) ? value.slice(0, 7) : fallback;
}
