import { useEffect, useState } from "react";

type Kind = "media" | "file";

export function parseFetchURLs(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of text.split(/[\s]+/)) {
    const u = part.trim();
    if (!/^https?:\/\//i.test(u)) continue;
    const key = u.replace(/\/+$/, "");
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(u);
  }
  return out;
}

type Props = {
  busy?: boolean;
  progress?: number;
  message?: string;
  initialUrl?: string;
  onClose: () => void;
  onFetch: (input: { urls: string[]; mode: "video" | "audio"; maxHeight: number | null }) => void;
  onImport: (urls: string[]) => void;
};

export function FetchModal({
  busy,
  progress = 0,
  message,
  initialUrl = "",
  onClose,
  onFetch,
  onImport,
}: Props) {
  const [kind, setKind] = useState<Kind>("media");
  const [url, setUrl] = useState(initialUrl);
  const [mode, setMode] = useState<"video" | "audio">("video");
  const [maxHeight, setMaxHeight] = useState("max");

  useEffect(() => {
    if (initialUrl.trim()) setUrl(initialUrl.trim());
  }, [initialUrl]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !busy) onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, busy]);

  const urls = parseFetchURLs(url);

  function submit() {
    if (!urls.length || busy) return;
    if (kind === "file") {
      onImport(urls);
      return;
    }
    onFetch({
      urls,
      mode,
      maxHeight: mode === "audio" ? null : maxHeight === "max" ? 0 : Number(maxHeight),
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
            <p className="meta">
              {kind === "media"
                ? "Paste one or more links (line or space separated). Highest quality by default; large videos compress automatically; duplicates are skipped."
                : "One or more direct file URLs (image, PDF, etc.)."}
            </p>
          </div>
          <button type="button" className="icon-btn" aria-label="Close" onClick={onClose} disabled={busy}>
            <i className="fa-solid fa-xmark" />
          </button>
        </header>
        <div className="note-modal-body stack">
          <div className="fetch-kind-tabs" role="tablist">
            <button
              type="button"
              role="tab"
              className={kind === "media" ? "active" : ""}
              aria-selected={kind === "media"}
              disabled={busy}
              onClick={() => setKind("media")}
            >
              Photos & video
            </button>
            <button
              type="button"
              role="tab"
              className={kind === "file" ? "active" : ""}
              aria-selected={kind === "file"}
              disabled={busy}
              onClick={() => setKind("file")}
            >
              Direct file URL
            </button>
          </div>
          <label>
            URL{urls.length > 1 ? `s (${urls.length})` : ""}
            <textarea
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder={
                kind === "file"
                  ? "https://example.com/photo.jpg\nhttps://…"
                  : "https://…\nhttps://… (one per line)"
              }
              rows={4}
              autoFocus
              disabled={busy}
            />
          </label>
          {kind === "media" && (
            <div className="row gap">
              <label style={{ flex: 1 }}>
                Save as
                <select
                  value={mode}
                  onChange={(e) => setMode(e.target.value as "video" | "audio")}
                  disabled={busy}
                >
                  <option value="video">Photos & video</option>
                  <option value="audio">Audio only (mp3)</option>
                </select>
              </label>
              <label style={{ flex: 1 }}>
                Max video height
                <select
                  value={maxHeight}
                  onChange={(e) => setMaxHeight(e.target.value)}
                  disabled={busy || mode === "audio"}
                >
                  <option value="max">Original (max)</option>
                  <option value="2160">4K</option>
                  <option value="1440">1440p</option>
                  <option value="1080">1080p</option>
                  <option value="720">720p</option>
                  <option value="480">480p</option>
                  <option value="360">360p (fastest)</option>
                </select>
              </label>
            </div>
          )}
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
          <button type="button" className="btn" disabled={busy || !urls.length} onClick={submit}>
            {busy
              ? `${pct}%`
              : kind === "file"
                ? urls.length > 1
                  ? `Import ${urls.length}`
                  : "Import to Drive"
                : urls.length > 1
                  ? `Fetch ${urls.length}`
                  : "Fetch to Drive"}
          </button>
        </footer>
      </div>
    </div>
  );
}
