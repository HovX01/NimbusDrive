import { useEffect, useState } from "react";

type Props = {
  busy?: boolean;
  progress?: number;
  message?: string;
  onClose: () => void;
  onFetch: (input: { url: string; mode: "video" | "audio"; maxHeight: number | null }) => void;
};

export function FetchModal({ busy, progress = 0, message, onClose, onFetch }: Props) {
  const [url, setUrl] = useState("");
  const [mode, setMode] = useState<"video" | "audio">("video");
  const [maxHeight, setMaxHeight] = useState("480");

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !busy) onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, busy]);

  function submit() {
    const trimmed = url.trim();
    if (!trimmed || busy) return;
    onFetch({
      url: trimmed,
      mode,
      maxHeight: mode === "audio" || !maxHeight ? null : Number(maxHeight),
    });
  }

  const pct = Math.max(0, Math.min(100, Math.round(progress)));

  return (
    <div className="modal-backdrop" role="presentation" onClick={() => !busy && onClose()}>
      <div
        className="modal note-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="fetch-modal-title"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="modal-head">
          <div>
            <h2 id="fetch-modal-title">Fetch from link</h2>
            <p className="meta">Native extractors (Cobalt-style). Default 480p. Supports TikTok, YouTube, X, Reddit, Facebook.</p>
          </div>
          <button type="button" className="icon-btn" aria-label="Close" onClick={onClose} disabled={busy}>
            <i className="fa-solid fa-xmark" />
          </button>
        </header>
        <div className="note-modal-body stack">
          <label>
            URL
            <textarea
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://…"
              rows={3}
              autoFocus
              disabled={busy}
            />
          </label>
          <div className="row gap">
            <label style={{ flex: 1 }}>
              Mode
              <select
                value={mode}
                onChange={(e) => setMode(e.target.value as "video" | "audio")}
                disabled={busy}
              >
                <option value="video">Video</option>
                <option value="audio">Audio (mp3)</option>
              </select>
            </label>
            <label style={{ flex: 1 }}>
              Max height
              <select
                value={maxHeight}
                onChange={(e) => setMaxHeight(e.target.value)}
                disabled={busy || mode === "audio"}
              >
                <option value="360">360p (fastest)</option>
                <option value="480">480p (recommended)</option>
                <option value="720">720p</option>
                <option value="1080">1080p (slow)</option>
                <option value="1440">1440p</option>
                <option value="2160">4K</option>
              </select>
            </label>
          </div>
          {busy && (
            <div className="fetch-progress" aria-live="polite">
              <div className="fetch-progress-meta">
                <span>{message || "Working…"}</span>
                <strong>{pct}%</strong>
              </div>
              <div className="fetch-progress-track">
                <div className="fetch-progress-bar" style={{ width: `${pct}%` }} />
              </div>
            </div>
          )}
        </div>
        <footer className="modal-foot row end gap">
          <button type="button" className="btn ghost" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="button" className="btn" disabled={busy || !url.trim()} onClick={submit}>
            {busy ? `${pct}%` : "Fetch to Drive"}
          </button>
        </footer>
      </div>
    </div>
  );
}
