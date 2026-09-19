import { useEffect, useState } from "react";
import {
  createS3Bucket,
  fetchS3BucketObjects,
  fetchS3Buckets,
  type S3Bucket,
  type S3Object,
} from "../api";
import { Portal } from "./Portal";

type Props = {
  token: string;
  onClose: () => void;
};

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(1)} ${units[unit]}`;
}

export function S3BucketsModal({ token, onClose }: Props) {
  const [buckets, setBuckets] = useState<S3Bucket[]>([]);
  const [loading, setLoading] = useState(true);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [openBucket, setOpenBucket] = useState<string | null>(null);
  const [objects, setObjects] = useState<S3Object[] | null>(null);
  const [objectsLoading, setObjectsLoading] = useState(false);
  const [uploadHint, setUploadHint] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    void refresh();
  }, [token]);

  async function refresh() {
    setLoading(true);
    setError("");
    try {
      const r = await fetchS3Buckets(token);
      setBuckets(r.items ?? []);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function handleCreate() {
    const trimmed = name.trim();
    if (!trimmed) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const bucket = await createS3Bucket(token, trimmed);
      setName("");
      setMessage(`Bucket "${bucket.name}" created. Upload to it with rclone or aws-cli.`);
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function toggleBucket(bucket: string) {
    if (openBucket === bucket) {
      setOpenBucket(null);
      setObjects(null);
      setUploadHint("");
      return;
    }
    setOpenBucket(bucket);
    setObjects(null);
    setObjectsLoading(true);
    setError("");
    try {
      const r = await fetchS3BucketObjects(token, bucket);
      setObjects(r.items ?? []);
      setUploadHint(r.upload);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setObjectsLoading(false);
    }
  }

  async function copyUploadCommand() {
    if (!uploadHint) return;
    try {
      await navigator.clipboard.writeText(uploadHint);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      window.prompt("Copy this command:", uploadHint);
    }
  }

  return (
    <Portal>
      <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <div
          className="modal note-modal settings-modal"
          role="dialog"
          aria-modal="true"
          aria-label="S3 buckets"
          onClick={(e) => e.stopPropagation()}
        >
          <header className="modal-head">
            <h2>S3 Buckets</h2>
            <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}>
              <i className="fa-solid fa-xmark" />
            </button>
          </header>
          <div className="note-modal-body stack">
            <section className="settings-section">
              <p className="meta">
                Each bucket is a top-level folder in your drive. Create one, then point your S3
                client at it to save files. Keys map to nested paths.
              </p>
              {error && <p className="error-text">{error}</p>}
              {message && <p className="meta">{message}</p>}
              <div className="row gap">
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="my-bucket"
                  onKeyDown={(e) => {
                    if (e.key === "Enter") void handleCreate();
                  }}
                />
                <button
                  type="button"
                  className="btn"
                  disabled={busy || !name.trim()}
                  onClick={() => void handleCreate()}
                >
                  Create Bucket
                </button>
              </div>
              <p className="meta">
                Lowercase letters, numbers, dots and hyphens; 3–63 characters.
              </p>
            </section>

            <section className="settings-section">
              <h3>Buckets</h3>
              {loading ? (
                <p className="meta">Loading…</p>
              ) : buckets.length === 0 ? (
                <p className="meta">No buckets yet — create one above.</p>
              ) : (
                <div className="stack">
                  {buckets.map((b) => (
                    <div key={b.name} className="stack">
                      <div className="row gap">
                        <button
                          type="button"
                          className="btn ghost compact"
                          aria-expanded={openBucket === b.name}
                          onClick={() => void toggleBucket(b.name)}
                        >
                          <i className="fa-solid fa-folder" />
                          {b.name}
                        </button>
                        <span className="meta">
                          {new Date(b.created_at).toLocaleDateString()}
                        </span>
                      </div>
                      {openBucket === b.name && (
                        <div className="stack">
                          {objectsLoading ? (
                            <p className="meta">Checking…</p>
                          ) : objects === null || objects.length === 0 ? (
                            <p className="meta">No objects in this bucket yet.</p>
                          ) : (
                            <table className="data-table">
                              <thead>
                                <tr>
                                  <th>Key</th>
                                  <th>Size</th>
                                  <th>Modified</th>
                                </tr>
                              </thead>
                              <tbody>
                                {objects.map((o) => (
                                  <tr key={o.key}>
                                    <td>{o.key}</td>
                                    <td>{formatSize(o.size)}</td>
                                    <td>{new Date(o.modified).toLocaleString()}</td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          )}
                          {uploadHint && (
                            <div className="row gap">
                              <code className="meta">{uploadHint}</code>
                              <button
                                type="button"
                                className="btn ghost compact"
                                onClick={() => void copyUploadCommand()}
                              >
                                {copied ? "Copied" : "Copy"}
                              </button>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </section>
          </div>
        </div>
      </div>
    </Portal>
  );
}
