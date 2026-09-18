import { useEffect, useState } from "react";
import { fetchFileBlob, fetchThumbBlob, mediaStreamUrl, type Node } from "../api";
import { extOf, formatBytes, kindLabel, previewKind, usesFullscreenViewer } from "../lib/files";
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
  onMediaStudio?: () => void;
  onEditVideo?: () => void;
  onDelete: () => void;
};

function likelyHEVC(name: string, mime: string) {
  const n = name.toLowerCase();
  const m = mime.toLowerCase();
  if (n.includes("screenrecording") || n.includes("screen-recording") || n.includes("screen_recording")) {
    return true;
  }
  if (extOf(name) === "mov" && (m.includes("quicktime") || m === "" || m === "application/octet-stream")) {
    return true;
  }
  return m.includes("hevc") || m.includes("h265");
}

function isHEIC(name: string, mime: string) {
  const m = mime.toLowerCase();
  const e = extOf(name);
  return e === "heic" || e === "heif" || m.includes("heic") || m.includes("heif");
}

function PreviewContent({
  token,
  node,
  kind,
  onDownload,
  onMediaStudio,
}: {
  token: string;
  node: Node;
  kind: ReturnType<typeof previewKind>;
  onDownload: () => void;
  onMediaStudio?: () => void;
}) {
  const [url, setUrl] = useState<string | null>(null);
  const [thumbUrl, setThumbUrl] = useState<string | null>(null);
  const [text, setText] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [buffering, setBuffering] = useState(false);
  const [playError, setPlayError] = useState("");
  const hevc = kind === "video" && likelyHEVC(node.name, node.mime_type);
  const heic = kind === "image" && isHEIC(node.name, node.mime_type);

  useEffect(() => {
    const urls: string[] = [];
    let cancelled = false;
    setLoading(true);
    setError("");
    setPlayError("");
    setBuffering(false);
    setText(null);
    setUrl(null);
    setThumbUrl(null);

    if (kind === "video" || kind === "audio") {
      if (kind === "video" && node.size < 1024) {
        setError("This video file is incomplete (stored size under 1 KB).");
        setLoading(false);
        return;
      }
      setUrl(mediaStreamUrl(token, node.id));
      setLoading(false);
      setBuffering(true);
      return;
    }

    if (kind === "image") {
      if (heic) {
        setError("HEIC photos need conversion — use Media Studio or download to view on Windows.");
        setLoading(false);
        return;
      }
      // Stream from server (local cache after first open) instead of downloading a huge blob in JS.
      setUrl(mediaStreamUrl(token, node.id));
      setLoading(false);
      void fetchThumbBlob(token, node.id)
        .then((thumb) => {
          if (cancelled) return;
          const u = URL.createObjectURL(thumb);
          urls.push(u);
          setThumbUrl(u);
        })
        .catch(() => {
          /* optional */
        });
      return;
    }

    (async () => {
      try {
        const blob = await fetchFileBlob(token, node.id);
        if (cancelled) return;
        if (kind === "text") {
          setText(await blob.text());
        } else if (kind === "pdf") {
          const objectUrl = URL.createObjectURL(blob);
          urls.push(objectUrl);
          setUrl(objectUrl);
        }
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
      for (const u of urls) URL.revokeObjectURL(u);
    };
  }, [token, node.id, node.size, kind, heic]);

  if (loading) {
    return (
      <div className="skeleton preview-skeleton" aria-busy="true">
        <span className="viewer-hint">Loading preview…</span>
      </div>
    );
  }
  if (error) {
    return (
      <div className="empty-panel viewer-empty">
        <p>Preview unavailable</p>
        <p className="meta">{error}</p>
        <div className="row gap" style={{ justifyContent: "center", flexWrap: "wrap" }}>
          <button type="button" className="btn" onClick={onDownload}>
            Download
          </button>
        </div>
      </div>
    );
  }

  if (kind === "image" && url) {
    return (
      <div className="viewer-image-wrap">
        {thumbUrl && (
          <img
            src={thumbUrl}
            alt=""
            className="viewer-media viewer-thumb-blur"
            aria-hidden
          />
        )}
        <img
          src={url}
          alt={node.name}
          className="viewer-media"
          onLoad={() => setThumbUrl(null)}
        />
      </div>
    );
  }

  if (kind === "video" && url) {
    if (playError) {
      return (
        <div className="empty-panel viewer-empty">
          <p>Can’t play this video in the browser</p>
          <p className="meta">
            {playError}
            {hevc
              ? " iPhone screen recordings are usually HEVC — Chrome on Windows often can’t decode them."
              : ""}
          </p>
          <div className="row gap" style={{ justifyContent: "center", flexWrap: "wrap" }}>
            {onMediaStudio && (
              <button type="button" className="btn" onClick={onMediaStudio}>
                Make playable (Media Studio)
              </button>
            )}
            <button type="button" className="btn ghost" onClick={onDownload}>
              Download
            </button>
          </div>
        </div>
      );
    }
    return (
      <div className="viewer-video-wrap">
        {buffering && (
          <p className="viewer-hint viewer-buffering">
            {hevc
              ? "Loading… If this stays blank, use Make playable (HEVC)."
              : "Loading from Telegram (first open may take a moment)…"}
          </p>
        )}
        {hevc && !buffering && onMediaStudio && (
          <p className="viewer-hint">
            Screen recording may need{" "}
            <button type="button" className="linkish" onClick={onMediaStudio}>
              Make playable
            </button>{" "}
            if it won’t start.
          </p>
        )}
        <video
          key={node.id}
          src={url}
          controls
          autoPlay
          playsInline
          preload="auto"
          className="viewer-media"
          onLoadStart={() => setBuffering(true)}
          onWaiting={() => setBuffering(true)}
          onPlaying={() => setBuffering(false)}
          onCanPlay={() => setBuffering(false)}
          onError={(e) => {
            const media = e.currentTarget;
            // MEDIA_ERR_SRC_NOT_SUPPORTED / decode — often HEVC; network blips shouldn't look permanent.
            const code = media.error?.code;
            if (code === 2) {
              // MEDIA_ERR_NETWORK — retry once by remounting isn't automatic; show soft message
              setPlayError("Network hiccup while streaming. Close and open again, or try Make playable.");
              return;
            }
            setPlayError(
              hevc
                ? "This looks like an HEVC screen recording."
                : "The browser can’t decode this video codec.",
            );
          }}
        />
      </div>
    );
  }

  if (kind === "audio" && url) {
    return (
      <div className="audio-wrap viewer-audio">
        <i className="fa-solid fa-music viewer-audio-ico" aria-hidden />
        <audio key={node.id} src={url} controls autoPlay preload="auto" />
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
  onMediaStudio,
  onEditVideo,
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
        <button type="button" className="icon-btn" aria-label="Rename" title="Rename" onClick={onRename}>
          <i className="fa-solid fa-pen" aria-hidden />
        </button>
        <button type="button" className="icon-btn" aria-label="Share" title="Share" onClick={onShare}>
          <i className="fa-solid fa-link" aria-hidden />
        </button>
        <button
          type="button"
          className="icon-btn"
          aria-label="Send to Telegram"
          title="Send to Telegram"
          onClick={onSendTelegram}
        >
          <i className="fa-solid fa-paper-plane" aria-hidden />
        </button>
        {onMediaStudio && (
          <button
            type="button"
            className="icon-btn"
            aria-label="Media Studio"
            title="Media Studio"
            onClick={onMediaStudio}
          >
            <i className="fa-solid fa-film" aria-hidden />
          </button>
        )}
        {(kind === "video" || kind === "image") && onEditVideo && (
          <button
            type="button"
            className="icon-btn"
            aria-label="Edit Media"
            title="Edit Media"
            onClick={onEditVideo}
          >
            <i className="fa-solid fa-scissors" aria-hidden />
          </button>
        )}
        <button type="button" className="icon-btn" aria-label="Download" title="Download" onClick={onDownload}>
          <i className="fa-solid fa-download" aria-hidden />
        </button>
        <button
          type="button"
          className="icon-btn danger"
          aria-label="Move to trash"
          title="Move to trash"
          onClick={onDelete}
        >
          <i className="fa-solid fa-trash" aria-hidden />
        </button>
        <span className="action-sep" aria-hidden />
        <button type="button" className="icon-btn" aria-label="Close preview" title="Close" onClick={onClose}>
          <i className="fa-solid fa-xmark" aria-hidden />
        </button>
      </div>
    </header>
  );

  const stage = (
    <PreviewContent
      token={token}
      node={node}
      kind={kind}
      onDownload={onDownload}
      onMediaStudio={onMediaStudio}
    />
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
              <div className="viewer-stage">{stage}</div>
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
          <div className="preview-stage">{stage}</div>
        </div>
      </div>
    </Portal>
  );
}
