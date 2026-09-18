import { useEffect } from "react";
import { formatBytes, previewKind, type PreviewKind } from "../lib/files";

export type UploadJob = {
  id: string;
  name: string;
  size: number;
  progress: number;
  status: "queued" | "uploading" | "processing" | "done" | "error";
  error?: string;
};

const KIND_ICON: Record<PreviewKind, string> = {
  folder: "fa-solid fa-folder",
  image: "fa-solid fa-image",
  video: "fa-solid fa-film",
  audio: "fa-solid fa-music",
  pdf: "fa-solid fa-file-pdf",
  text: "fa-solid fa-file-lines",
  archive: "fa-solid fa-file-zipper",
  file: "fa-solid fa-file",
};

type Props = {
  jobs: UploadJob[];
  onClose: () => void;
  onDismiss: (id: string) => void;
};

export function UploadPanel({ jobs, onClose, onDismiss }: Props) {
  if (!jobs.length) return null;

  const active = jobs.filter(
    (j) => j.status === "queued" || j.status === "uploading" || j.status === "processing",
  ).length;
  const done = jobs.filter((j) => j.status === "done").length;
  const failed = jobs.filter((j) => j.status === "error").length;
  const allSettled = active === 0;

  useEffect(() => {
    if (!allSettled || failed > 0) return;
    const timer = window.setTimeout(onClose, 2000);
    return () => window.clearTimeout(timer);
  }, [allSettled, failed, onClose]);

  return (
    <div className="upload-panel" role="status" aria-live="polite">
      <div className="upload-panel-head">
        <div className="upload-panel-title">
          <i
            className={`fa-solid ${
              allSettled
                ? failed
                  ? "fa-circle-exclamation"
                  : "fa-circle-check"
                : "fa-cloud-arrow-up"
            }`}
          />
          <div>
            <strong>
              {allSettled
                ? failed
                  ? `${failed} failed`
                  : "Upload complete"
                : `Uploading ${Math.min(done + failed + 1, jobs.length)} of ${jobs.length}`}
            </strong>
            <p className="meta">
              {allSettled
                ? `${done} done${failed ? ` · ${failed} error${failed === 1 ? "" : "s"}` : ""}`
                : `${active} in progress`}
            </p>
          </div>
        </div>
        <button type="button" className="icon-btn" aria-label="Close upload panel" onClick={onClose}>
          <i className="fa-solid fa-xmark" />
        </button>
      </div>

      <ul className="upload-list">
        {jobs.map((job) => {
          const kind = previewKind(job.name, "", false);
          return (
            <li key={job.id} className={`upload-row status-${job.status}`}>
              <div className="upload-file-ico" aria-hidden>
                <i className={KIND_ICON[kind]} />
              </div>
              <div className="upload-file-meta">
                <div className="upload-file-name truncate" title={job.name}>
                  {job.name}
                </div>
                <div className="meta">
                  {job.status === "error"
                    ? job.error || "Failed"
                    : job.status === "done"
                      ? `Saved · ${formatBytes(job.size)}`
                      : job.status === "processing"
                        ? `Storing in Nimbus · ${formatBytes(job.size)}`
                        : job.status === "uploading"
                          ? `${job.progress}% · ${formatBytes(job.size)}`
                          : `Queued · ${formatBytes(job.size)}`}
                </div>
                {(job.status === "uploading" ||
                  job.status === "queued" ||
                  job.status === "processing") && (
                  <div className="upload-bar" aria-hidden>
                    <div
                      className={`upload-bar-fill ${
                        job.status === "queued" || job.status === "processing" ? "indeterminate" : ""
                      }`}
                      style={job.status === "uploading" ? { width: `${job.progress}%` } : undefined}
                    />
                  </div>
                )}
              </div>
              <div className="upload-row-status">
                {(job.status === "uploading" || job.status === "processing") && (
                  <i className="fa-solid fa-spinner fa-spin" title={job.status === "processing" ? "Saving" : "Uploading"} />
                )}
                {job.status === "queued" && <i className="fa-regular fa-clock" />}
                {job.status === "done" && <i className="fa-solid fa-check" />}
                {job.status === "error" && <i className="fa-solid fa-triangle-exclamation" />}
                {allSettled && (
                  <button
                    type="button"
                    className="icon-btn tiny"
                    aria-label={`Dismiss ${job.name}`}
                    onClick={() => onDismiss(job.id)}
                  >
                    <i className="fa-solid fa-xmark" />
                  </button>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
