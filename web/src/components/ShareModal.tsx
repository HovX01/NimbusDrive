import { useState } from "react";
import type { ShareInfo } from "../api";
import { Portal } from "./Portal";

type Props = {
  fileName: string;
  busy: boolean;
  shares: ShareInfo[];
  onClose: () => void;
  onCreate: () => void;
  onRevoke: (shareId: string) => void;
};

export function ShareModal({ fileName, busy, shares, onClose, onCreate, onRevoke }: Props) {
  const [copied, setCopied] = useState<string | null>(null);

  async function copy(url: string) {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(url);
      window.setTimeout(() => setCopied(null), 2000);
    } catch {
      window.prompt("Copy this link:", url);
    }
  }

  return (
    <Portal>
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <div className="modal note-modal" role="dialog" aria-modal="true" aria-label="Share file" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head">
          <div>
            <p className="eyebrow">Share link</p>
            <h2>{fileName}</h2>
            <p className="meta">Anyone with the link can download this file.</p>
          </div>
          <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}>
            <i className="fa-solid fa-xmark" />
          </button>
        </header>
        <div className="note-modal-body">
          <button type="button" className="btn-create" disabled={busy} onClick={onCreate}>
            <i className="fa-solid fa-link" /> Create new link
          </button>
          {shares.length === 0 ? (
            <p className="meta" style={{ marginTop: "1rem" }}>
              No active links yet.
            </p>
          ) : (
            <ul className="share-list">
              {shares.map((s) => (
                <li key={s.link.id} className="share-row">
                  <code className="share-url truncate" title={s.url}>
                    {s.url}
                  </code>
                  <div className="share-row-actions">
                    <button type="button" className="btn ghost compact" onClick={() => copy(s.url)}>
                      {copied === s.url ? "Copied" : "Copy"}
                    </button>
                    <button type="button" className="btn danger-ghost compact" disabled={busy} onClick={() => onRevoke(s.link.id)}>
                      Revoke
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
    </Portal>
  );
}
