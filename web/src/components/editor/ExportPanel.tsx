import { useEffect, useState } from "react";
import {
  getEditExportStatus,
  startEditExport,
  type EditExportJobStatus,
  type EditProject,
  type ExportPreset,
} from "../../api";

type Props = {
  token: string;
  project: EditProject;
  onClose: () => void;
  onDone: () => void;
};

const PRESETS: { id: ExportPreset; label: string; icon: string }[] = [
  { id: "match", label: "Match original", icon: "fa-solid fa-equals" },
  { id: "1080p", label: "1080p H.264", icon: "fa-solid fa-display" },
  { id: "compressed", label: "Compressed H.265", icon: "fa-solid fa-box-archive" },
];

export function ExportPanel({ token, project, onClose, onDone }: Props) {
  const [preset, setPreset] = useState<ExportPreset>("match");
  const [job, setJob] = useState<EditExportJobStatus | null>(null);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState("");
  const running = job?.status === "queued" || job?.status === "running";

  useEffect(() => {
    if (!job || job.status === "done" || job.status === "error") return;
    let cancelled = false;
    const timer = window.setInterval(() => {
      getEditExportStatus(token, job.id)
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
  }, [job, onDone, token]);

  async function start() {
    setStarting(true);
    setError("");
    try {
      const { job_id } = await startEditExport(token, project.id, preset);
      setJob({
        id: job_id,
        project_id: project.id,
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
    <div className="export-backdrop" role="presentation" onClick={() => !running && onClose()}>
      <div className="export-panel" role="dialog" aria-modal="true" aria-label="Export video" onClick={(e) => e.stopPropagation()}>
        <header className="export-head">
          <div>
            <p className="editor-kicker">Export</p>
            <h2>{project.name}</h2>
          </div>
          <button type="button" className="editor-tool-btn" disabled={!!running} onClick={onClose} title="Close">
            <i className="fa-solid fa-xmark" />
          </button>
        </header>
        <div className="export-presets" role="radiogroup" aria-label="Export preset">
          {PRESETS.map((item) => (
            <button
              key={item.id}
              type="button"
              role="radio"
              aria-checked={preset === item.id}
              className={preset === item.id ? "active" : ""}
              disabled={!!running}
              onClick={() => setPreset(item.id)}
            >
              <i className={item.icon} />
              <span>{item.label}</span>
            </button>
          ))}
        </div>
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
            {job.status === "done" && job.node && <p className="meta">Saved as <strong>{job.node.name}</strong></p>}
            {job.status === "error" && <p className="error-text">{job.message}</p>}
          </div>
        )}
        <footer className="modal-foot row end gap">
          <button type="button" className="btn ghost" disabled={!!running} onClick={onClose}>
            {job?.status === "done" ? "Close" : "Cancel"}
          </button>
          {job?.status !== "done" && (
            <button type="button" className="btn" disabled={!!running || starting} onClick={() => void start()}>
              {starting || running ? "Exporting..." : "Export"}
            </button>
          )}
        </footer>
      </div>
    </div>
  );
}
