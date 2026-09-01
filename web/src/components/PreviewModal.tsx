import { useEffect, useState } from "react";
import { fetchFileBlob, fetchThumbBlob, type Node } from "../api";
import { formatBytes, kindLabel, previewKind, usesFullscreenViewer } from "../lib/files";
import { FileThumb } from "./FileThumb";
import { Portal } from "./Portal";

type Props = {
  token: string;
  node: Node;
  playlist: Node[];
  index: number;
  onNavigate: (index: number) => void;
  onClose: () => void;
  onDownload: () => void;
  onRename: () => void;
  onShare: () => void;
  onSendTelegram: () => void;
  onDelete: () => void;
};

function PreviewContent({
  token,
  node,
  kind,
  onDownload,
}: {
  token: string;
  node: Node;
  kind: ReturnType<typeof previewKind>;
  onDownload: () => void;
}) {
  const [url, setUrl] = useState<string | null>(null);
  const [text, setText] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [upgrading, setUpgrading] = useState(false);

  useEffect(() => {
    const urls: string[] = [];
    let cancelled = false;
    setLoading(true);
    setUpgrading(false);
    setError("");
    setText(null);
    setUrl(null);

    (async () => {
      try {
        if (kind === "image") {
          try {
            const thumb = await fetchThumbBlob(token, node.id);
            if (cancelled) return;
            const thumbUrl = URL.createObjectURL(thumb);
            urls.push(thumbUrl);
            setUrl(thumbUrl);
            setLoading(false);
            setUpgrading(true);
          } catch {
            /* full fetch below */
          }

          const full = await fetchFileBlob(token, node.id);
          if (cancelled) return;
          const fullUrl = URL.createObjectURL(full);
          urls.push(fullUrl);
          setUrl(fullUrl);
          setUpgrading(false);
          setLoading(false);
          return;
        }

        const blob = await fetchFileBlob(token, node.id);
        if (cancelled) return;
        if (kind === "text") {
          setText(await blob.text());
        } else if (kind === "video" || kind === "audio" || kind === "pdf") {
          const objectUrl = URL.createObjectURL(blob);
          urls.push(objectUrl);
          setUrl(objectUrl);
        }
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      } finally {
        if (!cancelled) {
          setLoading(false);
          setUpgrading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
      for (const u of urls) URL.revokeObjectURL(u);
    };
  }, [token, node.id, kind]);

  if (loading) {
    return <div className="skeleton preview-skeleton" aria-busy="true" />;
  }
  if (error) {
    return (
      <div className="empty-panel viewer-empty">
        <p>Preview unavailable</p>
        <p className="meta">{error}. You can still download the file.</p>
        <button type="button" className="btn" onClick={onDownload}>
          Download
        </button>
      </div>
    );
  }

  if (kind === "image" && url) {
    return (
      <>
        {upgrading && <p className="viewer-hint">Loading full quality…</p>}
        <img src={url} alt={node.name} className="viewer-media" />
      </>
    );
  }

  if (kind === "video" && url && node.size < 1024) {
    return (
      <div className="empty-panel viewer-empty">
        <p>This video file is incomplete</p>
        <p className="meta">
          Stored size is only {node.size} bytes. Delete it and upload the original again.
        </p>
        <button type="button" className="btn" onClick={onDownload}>
          Download anyway
        </button>
      </div>
    );
  }

  if (kind === "video" && url) {
    return <video key={node.id} src={url} controls autoPlay playsInline className="viewer-media" />;
  }

  if (kind === "audio" && url) {
    return (
      <div className="audio-wrap viewer-audio">
        <i className="fa-solid fa-music viewer-audio-ico" aria-hidden />
        <audio key={node.id} src={url} controls autoPlay />
      </div>
    );
  }

  if (kind === "pdf" && url) {
    return <iframe title={node.name} src={url} className="viewer-pdf" />;
  }

  if (kind === "text" && text !== null) {
    return <pre className="preview-text viewer-text">{text}</pre>;
  }

  return (
    <div className="empty-panel viewer-empty">
      <p>No inline preview for this type</p>
      <button type="button" className="btn" onClick={onDownload}>
        Download
      </button>
    </div>
  );
}

export function PreviewModal({
  token,
  node,
  playlist,
  index,
  onNavigate,
  onClose,
  onDownload,
  onRename,
  onShare,
  onSendTelegram,
  onDelete,
}: Props) {
  const kind = previewKind(node.name, node.mime_type, false);
  const fullscreen = usesFullscreenViewer(kind);
  const hasPrev = index > 0;
  const hasNext = index >= 0 && index < playlist.length - 1;
  const counter = playlist.length > 1 && index >= 0 ? `${index + 1} / ${playlist.length}` : null;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      const typing = tag === "INPUT" || tag === "TEXTAREA";
      if (e.key === "Escape") onClose();
      if (typing || playlist.length < 2) return;
      if (e.key === "ArrowLeft" && hasPrev) {
        e.preventDefault();
        onNavigate(index - 1);
      }
      if (e.key === "ArrowRight" && hasNext) {
        e.preventDefault();
        onNavigate(index + 1);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, onNavigate, index, hasPrev, hasNext, playlist.length]);

  const toolbar = (
    <header className={fullscreen ? "viewer-toolbar" : "preview-header"}>
      <div className="viewer-meta">
        <p className="eyebrow">{kindLabel(kind)}</p>
        <h2 className={fullscreen ? "viewer-title" : "preview-title"}>{node.name}</h2>
        <p className="meta">
          {formatBytes(node.size)}
          {counter ? ` · ${counter}` : ""}
        </p>
      </div>
      <div className={fullscreen ? "viewer-actions" : "preview-actions"}>
        <button type="button" className="btn ghost" onClick={onRename}>
          <i className="fa-solid fa-pen" /> <span className="btn-label">Rename</span>
        </button>
        <button type="button" className="btn ghost" onClick={onShare}>
          <i className="fa-solid fa-link" /> <span className="btn-label">Share</span>
        </button>
        <button type="button" className="btn ghost" onClick={onSendTelegram}>
          <i className="fa-solid fa-paper-plane" /> <span className="btn-label">Telegram</span>
        </button>
        <button type="button" className="btn ghost" onClick={onDownload}>
          <span className="btn-label">Download</span>
        </button>
        <button type="button" className="btn danger-ghost" onClick={onDelete}>
          <span className="btn-label">Move to trash</span>
        </button>
        <button type="button" className="icon-btn" aria-label="Close preview" onClick={onClose}>
          <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden>
            <path d="M2 2l10 10M12 2L2 12" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
          </svg>
        </button>
      </div>
    </header>
  );

  if (fullscreen) {
    return (
      <Portal>
      <div className="viewer-backdrop" role="presentation" onClick={onClose}>
        <div
          className="viewer-shell"
          role="dialog"
          aria-modal="true"
          aria-label={`Preview ${node.name}`}
          onClick={(e) => e.stopPropagation()}
        >
          {toolbar}
          <div className="viewer-body">
            {hasPrev && (
              <button
                type="button"
                className="viewer-nav prev"
                aria-label="Previous file"
                onClick={() => onNavigate(index - 1)}
              >
                <i className="fa-solid fa-chevron-left" />
              </button>
            )}
            <div className="viewer-stage">
              <PreviewContent token={token} node={node} kind={kind} onDownload={onDownload} />
            </div>
            {hasNext && (
              <button
                type="button"
                className="viewer-nav next"
                aria-label="Next file"
                onClick={() => onNavigate(index + 1)}
              >
                <i className="fa-solid fa-chevron-right" />
              </button>
            )}
          </div>
          {playlist.length > 1 && (
            <footer className="viewer-filmstrip" aria-label="Files in folder">
              {playlist.map((n, i) => (
                <button
                  key={n.id}
                  type="button"
                  className={`viewer-thumb ${i === index ? "active" : ""}`}
                  aria-label={n.name}
                  aria-current={i === index ? "true" : undefined}
                  onClick={() => onNavigate(i)}
                >
                  <FileThumb
                    token={token}
                    id={n.id}
                    name={n.name}
                    mime={n.mime_type}
                    isFolder={false}
                    compact
                  />
                </button>
              ))}
            </footer>
          )}
        </div>
      </div>
      </Portal>
    );
  }

  return (
    <Portal>
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <div
        className="modal preview-modal"
        role="dialog"
        aria-modal="true"
        aria-label={`Preview ${node.name}`}
        onClick={(e) => e.stopPropagation()}
      >
        {toolbar}
        {playlist.length > 1 && (
          <div className="preview-nav-bar">
            <button type="button" className="btn ghost compact" disabled={!hasPrev} onClick={() => onNavigate(index - 1)}>
              <i className="fa-solid fa-chevron-left" /> Previous
            </button>
            {counter && <span className="meta">{counter}</span>}
            <button type="button" className="btn ghost compact" disabled={!hasNext} onClick={() => onNavigate(index + 1)}>
              Next <i className="fa-solid fa-chevron-right" />
            </button>
          </div>
        )}
        <div className="preview-stage">
          <PreviewContent token={token} node={node} kind={kind} onDownload={onDownload} />
        </div>
      </div>
    </div>
    </Portal>
  );
}
