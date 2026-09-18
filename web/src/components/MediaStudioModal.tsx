import { useEffect, useState } from "react";
import {
  mediaJobStatus,
  startMediaJob,
  type MediaAction,
  type MediaJobStatus,
  type Node,
} from "../api";
import { formatBytes } from "../lib/files";
import { Portal } from "./Portal";

type Props = {
  token: string;
  node: Node;
  busy?: boolean;
  onClose: () => void;
  onDone: () => void;
};

const ACTIONS: {
  id: MediaAction;
  title: string;
  blurb: string;
  icon: string;
  tag: string;
}[] = [
  {
    id: "playable",
    title: "Make playable",
    blurb: "Convert to browser-safe H.264/AAC MP4 without changing the original file.",
    icon: "fa-solid fa-circle-play",
    tag: "Convert",
  },
  {
    id: "extract_audio",
    title: "Extract audio",
    blurb: "Pull the soundtrack out as an .m4a file for reuse in image/video edits.",
    icon: "fa-solid fa-music",
    tag: "Audio",
  },
  {
    id: "enhance",
    title: "Enhance 1080p",
    blurb: "Upscale, denoise, sharpen, and boost color for a cleaner sharing copy.",
    icon: "fa-solid fa-wand-magic-sparkles",
    tag: "Enhance",
  },
  {
    id: "compress",
    title: "Compress",
    blurb: "Create a smaller H.265 MP4 copy when you want to save space.",
    icon: "fa-solid fa-file-zipper",
    tag: "Compress",
  },
];

export function MediaStudioModal({ token, node, busy = false, onClose, onDone }: Props) {
  const [action, setAction] = useState<MediaAction>("playable");
  const [job, setJob] = useState<MediaJobStatus | null>(null);
  const [error, setError] = useState("");
  const [starting, setStarting] = useState(false);

  const running = job && (job.status === "queued" || job.status === "running");
  const selectedAction = ACTIONS.find((item) => item.id === action) ?? ACTIONS[0];

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !running) onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, running]);

  useEffect(() => {
    if (!job || job.status === "done" || job.status === "error") return;
    let cancelled = false;
    const timer = window.setInterval(() => {
      mediaJobStatus(token, job.id)
        .then((status) => {
          if (cancelled) return;
          setJob(status);
          if (status.status === "done") onDone();
        })
        .catch((e) => {
          if (!cancelled) setError((e as Error).message);
        });
    }, 900);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [job, token, onDone]);

  async function start() {
    setError("");
    setStarting(true);
    try {
      const { job_id } = await startMediaJob(token, node.id, action);
      setJob({
        id: job_id,
        action,
        status: "queued",
        phase: "queued",
        progress: 0,
        message: "Queued...",
      });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setStarting(false);
    }
  }

  return (
    <Portal>
      <div className="modal-backdrop" role="presentation" onClick={() => !running && onClose()}>
        <div
          className="modal media-studio-modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="media-studio-title"
          onClick={(e) => e.stopPropagation()}
        >
          <header className="media-studio-head">
            <div>
              <p className="eyebrow">Media Studio</p>
              <h2 id="media-studio-title">Create from Drive media</h2>
              <p className="meta">OpenCut-inspired panels for quick FFmpeg jobs. Results save back to Nimbus/Telegram.</p>
            </div>
            <button type="button" className="icon-btn" aria-label="Close" disabled={!!running} onClick={onClose}>
              <i className="fa-solid fa-xmark" />
            </button>
          </header>

          <div className="media-studio-body">
            <aside className="media-studio-source">
              <div className="media-studio-source-art">
                <i className="fa-solid fa-photo-film" />
              </div>
              <div className="media-studio-source-meta">
                <span className="media-action-tag">Source</span>
                <strong className="truncate">{node.name}</strong>
                <small>{formatBytes(node.size)}</small>
              </div>
              <div className="media-studio-mini-timeline" aria-hidden>
                <span />
                <span />
                <span />
                <i />
              </div>
            </aside>

            <section className="media-studio-workspace">
              <div className="media-studio-action-rail" role="radiogroup" aria-label="Media action">
                {ACTIONS.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    role="radio"
                    aria-checked={action === item.id}
                    className={`media-studio-action ${action === item.id ? "active" : ""}`}
                    disabled={!!running || busy}
                    onClick={() => setAction(item.id)}
                  >
                    <i className={item.icon} aria-hidden />
                    <span>{item.title}</span>
                  </button>
                ))}
              </div>

              <div className="media-studio-details">
                <span className="media-action-tag">{selectedAction.tag}</span>
                <i className={selectedAction.icon} aria-hidden />
                <h3>{selectedAction.title}</h3>
                <p>{selectedAction.blurb}</p>
                <div className="media-studio-output">
                  <span>Output</span>
                  <strong>{action === "extract_audio" ? "Audio .m4a" : "Video .mp4"}</strong>
                </div>
              </div>
            </section>

            {(error || job) && (
              <section className="media-studio-status" aria-live="polite">
                {error && <p className="error-text">{error}</p>}
                {job && (
                  <div className="media-progress">
                    <div className="row gap" style={{ justifyContent: "space-between" }}>
                      <span className="meta">{job.message || job.phase}</span>
                      <span className="meta">{Math.round(job.progress)}%</span>
                    </div>
                    <div className="fetch-progress-track">
                      <div className="fetch-progress-bar" style={{ width: `${Math.min(100, job.progress)}%` }} />
                    </div>
                    {job.status === "done" && job.node && (
                      <p className="meta">Saved as <strong>{job.node.name}</strong></p>
                    )}
                    {job.status === "error" && <p className="error-text">{job.message}</p>}
                  </div>
                )}
              </section>
            )}
          </div>

          <footer className="media-studio-foot">
            <span className="media-studio-hint">Server FFmpeg job · Telegram-backed output</span>
            <button type="button" className="btn ghost" disabled={!!running} onClick={onClose}>
              {job?.status === "done" ? "Close" : "Cancel"}
            </button>
            {job?.status !== "done" && (
              <button type="button" className="btn" disabled={!!running || starting || busy} onClick={() => void start()}>
                {starting || running ? "Working..." : "Run"}
              </button>
            )}
          </footer>
        </div>
      </div>
    </Portal>
  );
}
