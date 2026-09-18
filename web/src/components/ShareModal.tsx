import { useMemo, useState } from "react";
import type { ShareInfo } from "../api";
import { Portal } from "./Portal";

type Props = {
  fileName: string;
  busy: boolean;
  shares: ShareInfo[];
  onClose: () => void;
  onCreate: () => Promise<ShareInfo | null>;
  onRevoke: (shareId: string) => void;
};

function canUseSystemShare() {
  return typeof navigator !== "undefined" && typeof navigator.share === "function";
}

export function ShareModal({ fileName, busy, shares, onClose, onCreate, onRevoke }: Props) {
  const [copied, setCopied] = useState<string | null>(null);
  const [sharing, setSharing] = useState(false);
  const systemShare = useMemo(() => canUseSystemShare(), []);

  async function copy(url: string) {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(url);
      window.setTimeout(() => setCopied(null), 2000);
    } catch {
      window.prompt("Copy this link:", url);
    }
  }

  async function ensureUrl(): Promise<string | null> {
    if (shares[0]?.url) return shares[0].url;
    const created = await onCreate();
    return created?.url ?? null;
  }

  async function shareSystem() {
    setSharing(true);
    try {
      const url = await ensureUrl();
      if (!url) return;
      if (systemShare) {
        try {
          await navigator.share({ title: fileName, text: fileName, url });
          return;
        } catch (e) {
          if ((e as Error).name === "AbortError") return;
        }
      }
      await copy(url);
    } finally {
      setSharing(false);
    }
  }

  async function openAppShare(kind: "telegram" | "whatsapp" | "messages") {
    setSharing(true);
    try {
      const url = await ensureUrl();
      if (!url) return;
      const text = `${fileName}\n${url}`;
      let href = "";
      if (kind === "telegram") {
        href = `https://t.me/share/url?url=${encodeURIComponent(url)}&text=${encodeURIComponent(fileName)}`;
      } else if (kind === "whatsapp") {
        href = `https://wa.me/?text=${encodeURIComponent(text)}`;
      } else {
        // iOS Messages / SMS — also works as a mailto fallback on desktop.
        const sms = `sms:?&body=${encodeURIComponent(text)}`;
        href = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent) ? sms : `mailto:?subject=${encodeURIComponent(fileName)}&body=${encodeURIComponent(text)}`;
      }
      window.open(href, "_blank", "noopener,noreferrer");
    } finally {
      setSharing(false);
    }
  }

  const disabled = busy || sharing;

  return (
    <Portal>
      <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <div
          className="modal note-modal share-sheet-modal"
          role="dialog"
          aria-modal="true"
          aria-label="Share file"
          onClick={(e) => e.stopPropagation()}
        >
          <header className="modal-head">
            <div>
              <p className="eyebrow">Share</p>
              <h2>{fileName}</h2>
              <p className="meta">Send via your apps — no need to copy the link by hand.</p>
            </div>
            <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}>
              <i className="fa-solid fa-xmark" />
            </button>
          </header>

          <div className="note-modal-body share-sheet-body">
            <div className="share-apps" role="list">
              {systemShare && (
                <button
                  type="button"
                  className="share-app"
                  role="listitem"
                  disabled={disabled}
                  onClick={() => void shareSystem()}
                >
                  <span className="share-app-ico system">
                    <i className="fa-solid fa-arrow-up-from-bracket" aria-hidden />
                  </span>
                  <span>Share via…</span>
                </button>
              )}
              <button
                type="button"
                className="share-app"
                role="listitem"
                disabled={disabled}
                onClick={() => void openAppShare("messages")}
              >
                <span className="share-app-ico messages">
                  <i className="fa-solid fa-comment" aria-hidden />
                </span>
                <span>Messages</span>
              </button>
              <button
                type="button"
                className="share-app"
                role="listitem"
                disabled={disabled}
                onClick={() => void openAppShare("whatsapp")}
              >
                <span className="share-app-ico whatsapp">
                  <i className="fa-brands fa-whatsapp" aria-hidden />
                </span>
                <span>WhatsApp</span>
              </button>
              <button
                type="button"
                className="share-app"
                role="listitem"
                disabled={disabled}
                onClick={() => void openAppShare("telegram")}
              >
                <span className="share-app-ico telegram">
                  <i className="fa-brands fa-telegram" aria-hidden />
                </span>
                <span>Telegram</span>
              </button>
              <button
                type="button"
                className="share-app"
                role="listitem"
                disabled={disabled}
                onClick={() => {
                  void (async () => {
                    setSharing(true);
                    try {
                      const url = await ensureUrl();
                      if (url) await copy(url);
                    } finally {
                      setSharing(false);
                    }
                  })();
                }}
              >
                <span className="share-app-ico copy">
                  <i className="fa-solid fa-link" aria-hidden />
                </span>
                <span>{copied ? "Copied" : "Copy link"}</span>
              </button>
            </div>

            <p className="meta share-sheet-hint">
              Nimbus creates a download link once, then opens your phone’s share targets.
            </p>

            {shares.length > 0 && (
              <ul className="share-list">
                {shares.map((s) => (
                  <li key={s.link.id} className="share-row">
                    <code className="share-url truncate" title={s.url}>
                      {s.url}
                    </code>
                    <div className="share-row-actions">
                      <button type="button" className="btn ghost compact" disabled={disabled} onClick={() => void copy(s.url)}>
                        {copied === s.url ? "Copied" : "Copy"}
                      </button>
                      <button
                        type="button"
                        className="btn danger-ghost compact"
                        disabled={disabled}
                        onClick={() => onRevoke(s.link.id)}
                      >
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
