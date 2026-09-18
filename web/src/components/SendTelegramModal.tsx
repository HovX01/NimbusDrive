import { useEffect, useMemo, useState } from "react";
import { listBots, listContacts, type TelegramContact } from "../api";
import { ContactAvatar } from "./ContactAvatar";
import { Portal } from "./Portal";

type Props = {
  token: string;
  fileName: string;
  busy: boolean;
  error?: string;
  onClose: () => void;
  onSend: (contact: TelegramContact) => void;
};

function contactHandle(c: TelegramContact) {
  if (c.username) return `@${c.username}`;
  return c.display_name;
}

function matchesQuery(c: TelegramContact, query: string) {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return (
    c.display_name.toLowerCase().includes(q) ||
    (c.username?.toLowerCase().includes(q) ?? false) ||
    (c.first_name?.toLowerCase().includes(q) ?? false)
  );
}

export function SendTelegramModal({ token, fileName, busy, error, onClose, onSend }: Props) {
  const [query, setQuery] = useState("");
  const [contacts, setContacts] = useState<TelegramContact[]>([]);
  const [bots, setBots] = useState<TelegramContact[]>([]);
  const [loading, setLoading] = useState(true);
  const [listError, setListError] = useState("");
  const [selected, setSelected] = useState<TelegramContact | null>(null);

  useEffect(() => {
    let cancelled = false;
    const timer = window.setTimeout(() => {
      setLoading(true);
      setListError("");
      Promise.all([listContacts(token, query), listBots(token)])
        .then(([people, botRes]) => {
          if (cancelled) return;
          setContacts(people.items ?? []);
          setBots(
            (botRes.items ?? [])
              .filter((b) => b.allowed)
              .map((b) => ({
                id: b.id,
                username: b.username,
                first_name: b.display_name,
                display_name: b.display_name,
                has_avatar: b.has_avatar,
                is_bot: true,
              })),
          );
        })
        .catch((e) => {
          if (!cancelled) {
            setListError((e as Error).message);
            setContacts([]);
            setBots([]);
          }
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, query ? 250 : 0);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [token, query]);

  const recipients = useMemo(() => {
    const seen = new Set<number>();
    const out: TelegramContact[] = [];
    // Allowed bots first (Telegram lets you DM bots the same way as people).
    for (const b of bots) {
      if (!matchesQuery(b, query) || seen.has(b.id)) continue;
      seen.add(b.id);
      out.push(b);
    }
    for (const c of contacts) {
      if (seen.has(c.id)) continue;
      seen.add(c.id);
      out.push(c);
    }
    return out;
  }, [bots, contacts, query]);

  return (
    <Portal>
      <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <div
          className="modal note-modal send-telegram-modal"
          role="dialog"
          aria-modal="true"
          aria-label="Send to Telegram"
          onClick={(e) => e.stopPropagation()}
        >
          <header className="modal-head">
            <div>
              <p className="eyebrow">Send to Telegram</p>
              <h2>{fileName}</h2>
              <p className="meta">
                Pick a person or an <strong>allowed bot</strong> — same as sending a file in Telegram.
              </p>
            </div>
            <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}>
              <i className="fa-solid fa-xmark" />
            </button>
          </header>
          <div className="note-modal-body send-telegram-body">
            {busy && (
              <div className="send-telegram-overlay" role="status" aria-live="polite">
                <i className="fa-solid fa-spinner fa-spin" aria-hidden />
                <p>Sending to {selected?.display_name ?? "chat"}…</p>
                <span className="meta">This may take a moment for large files.</span>
              </div>
            )}
            <label className="send-telegram-search">
              <i className="fa-solid fa-magnifying-glass" aria-hidden />
              <input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search people or bots"
                autoFocus
              />
            </label>
            {listError && <p className="banner error compact">{listError}</p>}
            {error && <p className="banner error compact">{error}</p>}
            {loading ? (
              <p className="meta">Loading…</p>
            ) : recipients.length === 0 ? (
              <p className="meta">
                No matches. Allow a bot in Settings, or open it in Telegram (tap Start) first.
              </p>
            ) : (
              <ul className="send-telegram-list">
                {recipients.map((c) => (
                  <li key={c.id}>
                    <button
                      type="button"
                      className={`send-telegram-row${selected?.id === c.id ? " selected" : ""}`}
                      disabled={busy}
                      onClick={() => setSelected(c)}
                    >
                      <ContactAvatar token={token} contact={c} />
                      <span className="send-telegram-meta">
                        <span className="row gap" style={{ alignItems: "center" }}>
                          <span className="truncate">{c.display_name}</span>
                          {c.is_bot && <span className="send-bot-badge">Bot</span>}
                        </span>
                        <span className="meta truncate">{contactHandle(c)}</span>
                      </span>
                      <i className="fa-solid fa-chevron-right send-telegram-icon" />
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
          {selected && !busy && (
            <footer className="modal-foot send-telegram-foot">
              <p className="send-telegram-confirm">
                Send <strong>{fileName}</strong> to{" "}
                <strong>
                  {selected.display_name}
                  {selected.is_bot ? " (bot)" : ""}
                </strong>
                ?
              </p>
              <div className="row end gap">
                <button type="button" className="btn ghost" onClick={() => setSelected(null)}>
                  Cancel
                </button>
                <button type="button" className="btn-create" onClick={() => onSend(selected)}>
                  <i className="fa-solid fa-paper-plane" /> Send
                </button>
              </div>
            </footer>
          )}
        </div>
      </div>
    </Portal>
  );
}
