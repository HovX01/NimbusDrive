import { useEffect, useState } from "react";
import { listFiles, type Node } from "../api";
import { Portal } from "./Portal";

type Crumb = { id: string; name: string };

type Props = {
  token: string;
  busy: boolean;
  count: number;
  excludeIds: string[];
  onClose: () => void;
  onConfirm: (parentId: string) => void;
};

export function MoveModal({ token, busy, count, excludeIds, onClose, onConfirm }: Props) {
  const [trail, setTrail] = useState<Crumb[]>([{ id: "root", name: "My Drive" }]);
  const [folders, setFolders] = useState<Node[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const parentId = trail[trail.length - 1]?.id ?? "root";

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    listFiles(token, parentId)
      .then((r) => {
        if (cancelled) return;
        setFolders((r.items ?? []).filter((n) => n.type === "folder" && !excludeIds.includes(n.id)));
      })
      .catch((e) => {
        if (!cancelled) setError((e as Error).message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token, parentId, excludeIds]);

  return (
    <Portal>
      <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <div
          className="modal note-modal move-modal"
          role="dialog"
          aria-modal="true"
          aria-label="Move items"
          onClick={(e) => e.stopPropagation()}
        >
          <header className="modal-head">
            <div>
              <p className="eyebrow">Move to folder</p>
              <h2>
                {count} item{count === 1 ? "" : "s"}
              </h2>
              <p className="meta">Choose a destination folder.</p>
            </div>
            <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}>
              <i className="fa-solid fa-xmark" />
            </button>
          </header>
          <div className="note-modal-body move-modal-body">
            <div className="crumbs move-crumbs" aria-label="Folder path">
              {trail.map((c, i) => (
                <button key={c.id} type="button" className="crumb" onClick={() => setTrail(trail.slice(0, i + 1))}>
                  {c.name}
                </button>
              ))}
            </div>
            {error && <p className="banner error compact">{error}</p>}
            {loading ? (
              <p className="meta">Loading folders…</p>
            ) : folders.length === 0 ? (
              <p className="meta">No subfolders here.</p>
            ) : (
              <ul className="move-folder-list">
                {folders.map((f) => (
                  <li key={f.id}>
                    <button type="button" className="move-folder-row" onClick={() => setTrail([...trail, { id: f.id, name: f.name }])}>
                      <i className="fa-solid fa-folder" />
                      <span className="truncate">{f.name}</span>
                      <i className="fa-solid fa-chevron-right move-folder-chevron" />
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <footer className="modal-foot row end gap">
            <button type="button" className="btn ghost" onClick={onClose}>
              Cancel
            </button>
            <button type="button" className="btn-create" disabled={busy} onClick={() => onConfirm(parentId)}>
              Move here
            </button>
          </footer>
        </div>
      </div>
    </Portal>
  );
}
