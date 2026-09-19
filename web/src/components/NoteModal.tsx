import { useState } from "react";
import { Bold, Italic, Link2, List } from "lucide-react";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import { Input } from "./ui/input";
import { Textarea } from "./ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./ui/tooltip";

type Props = {
  busy?: boolean;
  onClose: () => void;
  onSave: (name: string, body: string) => void;
};

function suggestName(body: string): string {
  const t = body.trim();
  if (!t) return "note.txt";
  if (/^https?:\/\/\S+$/i.test(t)) {
    try {
      const host = new URL(t).hostname.replace(/^www\./, "");
      return `${host || "link"}.txt`;
    } catch {
      /* fall through */
    }
  }
  const first = t
    .split(/\r?\n/)[0]
    .replace(/^#+\s*/, "")
    .slice(0, 48)
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, "")
    .trim();
  if (first) return first.toLowerCase().endsWith(".txt") ? first : `${first}.txt`;
  const stamp = new Date().toISOString().slice(0, 16).replace("T", "_").replace(":", "");
  return `note_${stamp}.txt`;
}

function wrapSelection(text: string, start: number, end: number, before: string, after = before) {
  const selected = text.slice(start, end);
  const next = text.slice(0, start) + before + selected + after + text.slice(end);
  const cursor = start + before.length + selected.length + after.length;
  return { next, cursor };
}

function prefixLines(text: string, start: number, end: number, prefix: string) {
  const lineStart = text.lastIndexOf("\n", start - 1) + 1;
  const lineEnd = text.indexOf("\n", end);
  const blockEnd = lineEnd === -1 ? text.length : lineEnd;
  const block = text.slice(lineStart, blockEnd);
  const lines = block.split("\n").map((line) => (line.startsWith(prefix) ? line : `${prefix}${line}`));
  const next = text.slice(0, lineStart) + lines.join("\n") + text.slice(blockEnd);
  return { next, cursor: blockEnd + (lines.join("\n").length - block.length) };
}

export function NoteModal({ busy, onClose, onSave }: Props) {
  const [name, setName] = useState("");
  const [body, setBody] = useState("");
  const [nameTouched, setNameTouched] = useState(false);

  function applyEdit(edit: { next: string; cursor: number }) {
    setBody(edit.next);
    if (!nameTouched) setName(suggestName(edit.next).replace(/\.txt$/i, ""));
  }

  function format(action: "bold" | "italic" | "list" | "link") {
    const el = document.getElementById("note-body") as HTMLTextAreaElement | null;
    const start = el?.selectionStart ?? body.length;
    const end = el?.selectionEnd ?? body.length;
    if (action === "list") {
      applyEdit(prefixLines(body, start, end, "- "));
      return;
    }
    if (action === "link") {
      applyEdit(wrapSelection(body, start, end, "[", "](https://)"));
      return;
    }
    const mark = action === "bold" ? "**" : "*";
    applyEdit(wrapSelection(body, start, end, mark, mark));
  }

  function submit() {
    const text = body.trim();
    if (!text || busy) return;
    const fileName = (name.trim() || suggestName(text)).replace(/[<>:"/\\|?*]/g, "_");
    const withExt = /\.[a-z0-9]+$/i.test(fileName) ? fileName : `${fileName}.txt`;
    onSave(withExt, text);
  }

  const words = body.trim() ? body.trim().split(/\s+/).length : 0;

  const tools = [
    { id: "bold", icon: Bold, tip: "Bold" },
    { id: "italic", icon: Italic, tip: "Italic" },
    { id: "list", icon: List, tip: "Bullet list" },
    { id: "link", icon: Link2, tip: "Link" },
  ] as const;

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-[620px]">
        <DialogHeader className="space-y-3 px-5 pb-3 pt-5">
          <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">New note</p>
          <DialogTitle>
            <Input
              value={name}
              onChange={(e) => {
                setNameTouched(true);
                setName(e.target.value);
              }}
              placeholder="Untitled"
              autoComplete="off"
              aria-label="Note title"
              data-slot="note-title"
              className="border-0 bg-transparent px-0 text-2xl font-semibold tracking-tight shadow-none focus-visible:ring-0"
            />
          </DialogTitle>
        </DialogHeader>

        <TooltipProvider>
          <div className="flex items-center gap-0.5 border-y bg-muted/40 px-4 py-1.5" role="toolbar" aria-label="Formatting">
            {tools.map((t) => (
              <Tooltip key={t.id}>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon-sm" onClick={() => format(t.id)}>
                    <t.icon className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t.tip}</TooltipContent>
              </Tooltip>
            ))}
            <span className="ml-auto hidden text-xs text-muted-foreground sm:block">
              Markdown · Ctrl+Enter to save
            </span>
          </div>
        </TooltipProvider>

        <div className="p-2">
          <Textarea
            id="note-body"
            value={body}
            onChange={(e) => {
              const v = e.target.value;
              setBody(v);
              if (!nameTouched) setName(suggestName(v).replace(/\.txt$/i, ""));
            }}
            onKeyDown={(e) => {
              if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
                e.preventDefault();
                submit();
              }
            }}
            placeholder="Start writing, or paste a link…"
            autoFocus
            rows={10}
            className="min-h-[240px] resize-none border-0 shadow-none focus-visible:ring-0"
          />
        </div>

        <DialogFooter className="items-center gap-2 border-t bg-muted/30 px-5 py-3.5 sm:justify-between">
          <span className="text-xs text-muted-foreground">
            {words > 0 ? `${words} word${words === 1 ? "" : "s"}` : "Plain text"}
          </span>
          <div className="flex gap-2">
            <Button variant="outline" onClick={onClose} disabled={busy}>
              Cancel
            </Button>
            <Button disabled={busy || !body.trim()} onClick={submit}>
              Save to folder
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
