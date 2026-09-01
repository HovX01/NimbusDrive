import { useEffect, useRef, useState } from "react";
import { Portal } from "./Portal";

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
  const editorRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  function applyEdit(edit: { next: string; cursor: number }) {
    setBody(edit.next);
    if (!nameTouched) setName(suggestName(edit.next).replace(/\.txt$/i, ""));
    requestAnimationFrame(() => {
      const el = editorRef.current;
      if (!el) return;
      el.focus();
      el.setSelectionRange(edit.cursor, edit.cursor);
    });
  }

  function format(action: "bold" | "italic" | "list" | "link") {
    const el = editorRef.current;
    if (!el) return;
    const start = el.selectionStart;
    const end = el.selectionEnd;
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

  return (
    <Portal>
      <div className="modal-backdrop note-editor-backdrop" role="presentation" onClick={onClose}>
        <div
          className="modal note-editor-modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="note-modal-title"
          onClick={(e) => e.stopPropagation()}
        >
          <header className="note-editor-head">
            <div className="note-editor-head-main">
              <p className="eyebrow">New note</p>
              <input
                id="note-modal-title"
                className="note-editor-title"
                value={name}
                onChange={(e) => {
                  setNameTouched(true);
                  setName(e.target.value);
                }}
                placeholder="Untitled"
                autoComplete="off"
                aria-label="Note title"
              />
            </div>
            <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}>
              <i className="fa-solid fa-xmark" />
            </button>
          </header>

          <div className="note-editor-toolbar" role="toolbar" aria-label="Formatting">
            <button type="button" className="note-tool" title="Bold" onClick={() => format("bold")}>
              <i className="fa-solid fa-bold" />
            </button>
            <button type="button" className="note-tool" title="Italic" onClick={() => format("italic")}>
              <i className="fa-solid fa-italic" />
            </button>
            <button type="button" className="note-tool" title="Bullet list" onClick={() => format("list")}>
              <i className="fa-solid fa-list-ul" />
            </button>
            <button type="button" className="note-tool" title="Link" onClick={() => format("link")}>
              <i className="fa-solid fa-link" />
            </button>
            <span className="note-editor-hint meta">Markdown · Ctrl+Enter to save</span>
          </div>

          <div className="note-editor-body">
            <textarea
              ref={editorRef}
              className="note-editor-input"
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
            />
          </div>

          <footer className="note-editor-foot">
            <span className="meta note-editor-meta">{words > 0 ? `${words} word${words === 1 ? "" : "s"}` : "Plain text"}</span>
            <div className="row gap">
              <button type="button" className="btn ghost" onClick={onClose} disabled={busy}>
                Cancel
              </button>
              <button type="button" className="btn-create" disabled={busy || !body.trim()} onClick={submit}>
                Save to folder
              </button>
            </div>
          </footer>
        </div>
      </div>
    </Portal>
  );
}
