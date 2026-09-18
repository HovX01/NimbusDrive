import { useEffect, useRef, useState } from "react";
import {
  addSocialCookieConnection,
  disconnectSocialConnection,
  fetchStorageSettings,
  listBots,
  listSocialConnections,
  setBotAllowed,
  setActiveSocialConnection,
  setSocialConnectionEnabled,
  type SocialConnection,
  type SocialProvider,
  type TelegramBot,
} from "../api";
import { Portal } from "./Portal";

type Props = {
  token: string;
  onClose: () => void;
};

export function SettingsModal({ token, onClose }: Props) {
  const [apiKey, setApiKey] = useState("");
  const [apiBase, setApiBase] = useState("");
  const [bots, setBots] = useState<TelegramBot[]>([]);
  const [botsLoading, setBotsLoading] = useState(true);
  const [botsError, setBotsError] = useState("");
  const [busyId, setBusyId] = useState<number | null>(null);
  const [social, setSocial] = useState<SocialConnection[]>([]);
  const [socialLoading, setSocialLoading] = useState(true);
  const [socialError, setSocialError] = useState("");
  const [socialBusy, setSocialBusy] = useState<SocialProvider | null>(null);
  const [connectorReady, setConnectorReady] = useState(false);
  const [cookieForms, setCookieForms] = useState<Record<string, { displayName: string; file: File | null }>>({});
  const [cookieBusy, setCookieBusy] = useState<SocialProvider | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState<string | null>(null);
  const connectorTimeout = useRef<number | null>(null);

  async function refreshSocial() {
    const connections = await listSocialConnections(token);
    setSocial(connections.items ?? []);
  }

  useEffect(() => {
    let cancelled = false;
    fetchStorageSettings(token)
      .then((s) => {
        if (cancelled) return;
        setApiKey(s.api_key);
        setApiBase(s.api_base);
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
  }, [token]);

  useEffect(() => {
    let cancelled = false;
    setSocialLoading(true);
    setSocialError("");
    refreshSocial()
      .then(() => {
        if (cancelled) return;
      })
      .catch((e) => {
        if (!cancelled) setSocialError((e as Error).message);
      })
      .finally(() => {
        if (!cancelled) setSocialLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const provider = params.get("social");
    if (!provider) return;
    const status = params.get("status");
    if (status === "connected") {
      setCopied(`${provider} connected`);
      refreshSocial().catch(() => undefined);
    } else if (status === "error") {
      setSocialError(params.get("message") || `${provider} connection failed`);
    }
    window.history.replaceState({}, "", window.location.pathname);
    window.setTimeout(() => setCopied(null), 2500);
  }, [token]);

  useEffect(() => {
    function clearConnectorTimeout() {
      if (connectorTimeout.current !== null) {
        window.clearTimeout(connectorTimeout.current);
        connectorTimeout.current = null;
      }
    }

    async function onMessage(event: MessageEvent) {
      if (event.source !== window) return;
      const data = event.data as NimbusConnectorMessage | undefined;
      if (!data || typeof data !== "object") return;
      if (data.type === "NIMBUS_SOCIAL_CONNECTOR_READY") {
        setConnectorReady(true);
        return;
      }
      if (data.type !== "NIMBUS_SOCIAL_CONNECT_RESULT" || !isSocialProvider(data.provider)) return;

      clearConnectorTimeout();
      const provider = data.provider;
      if (!data.ok || !data.cookies) {
        setSocialBusy(null);
        setSocialError(data.error || `${providerLabel(provider)} did not return a logged-in browser session.`);
        return;
      }

      setSocialBusy(provider);
      setSocialError("");
      try {
        const cookies = new Blob([data.cookies], { type: "text/plain" });
        await addSocialCookieConnection(token, provider, data.label || `${providerLabel(provider)} browser session`, cookies);
        await refreshSocial();
        setCopied(`${providerLabel(provider)} connected`);
        window.setTimeout(() => setCopied(null), 2500);
      } catch (e) {
        setSocialError((e as Error).message);
      } finally {
        setSocialBusy(null);
      }
    }

    window.addEventListener("message", onMessage);
    window.postMessage({ type: "NIMBUS_SOCIAL_CONNECTOR_PING" }, window.location.origin);
    return () => {
      clearConnectorTimeout();
      window.removeEventListener("message", onMessage);
    };
  }, [token]);

  useEffect(() => {
    let cancelled = false;
    setBotsLoading(true);
    setBotsError("");
    listBots(token)
      .then((r) => {
        if (!cancelled) setBots(r.items ?? []);
      })
      .catch((e) => {
        if (!cancelled) setBotsError((e as Error).message);
      })
      .finally(() => {
        if (!cancelled) setBotsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  async function copy(text: string, label: string) {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(label);
      window.setTimeout(() => setCopied(null), 2000);
    } catch {
      window.prompt("Copy:", text);
    }
  }

  async function toggleBot(bot: TelegramBot) {
    setBusyId(bot.id);
    setBotsError("");
    try {
      const updated = await setBotAllowed(token, bot.id, !bot.allowed);
      setBots((prev) => prev.map((b) => (b.id === updated.id ? updated : b)));
    } catch (e) {
      setBotsError((e as Error).message);
    } finally {
      setBusyId(null);
    }
  }

  function connectProvider(provider: SocialProvider) {
    setSocialBusy(provider);
    setSocialError("");
    if (!connectorReady) {
      const opened = window.open(providerLoginURL(provider), "_blank", "noopener,noreferrer");
      if (!opened) {
        window.location.href = providerLoginURL(provider);
        return;
      }
      setSocialBusy(null);
      setSocialError(
        "Install Nimbus Social Connector from web/public/social-connector once, then Connect can finish automatically. You can still upload cookies.txt below.",
      );
      return;
    }
    window.postMessage({ type: "NIMBUS_SOCIAL_CONNECT", provider }, window.location.origin);
    connectorTimeout.current = window.setTimeout(() => {
      setSocialBusy(null);
      setSocialError(`Still waiting for ${providerLabel(provider)} login. Finish login in the opened tab, then click Connect again if needed.`);
    }, 5 * 60 * 1000 + 1000);
  }

  async function addCookieAccount(provider: SocialProvider) {
    const form = cookieForms[provider] ?? { displayName: "", file: null };
    if (!form.file) {
      setSocialError(`Choose a ${providerLabel(provider)} cookies.txt file first.`);
      return;
    }
    setCookieBusy(provider);
    setSocialError("");
    try {
      await addSocialCookieConnection(token, provider, form.displayName, form.file);
      setCookieForms((prev) => ({ ...prev, [provider]: { displayName: "", file: null } }));
      await refreshSocial();
    } catch (e) {
      setSocialError((e as Error).message);
    } finally {
      setCookieBusy(null);
    }
  }

  function updateCookieForm(provider: SocialProvider, patch: Partial<{ displayName: string; file: File | null }>) {
    setCookieForms((prev) => {
      const nextForm = prev[provider] ?? { displayName: "", file: null };
      return {
        ...prev,
        [provider]: { ...nextForm, ...patch },
      };
    });
  }

  async function toggleSocial(conn: SocialConnection) {
    setSocialBusy(conn.provider);
    setSocialError("");
    try {
      const updated = await setSocialConnectionEnabled(token, conn.provider, !conn.enabled);
      setSocial((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
    } catch (e) {
      setSocialError((e as Error).message);
    } finally {
      setSocialBusy(null);
    }
  }

  async function activateSocial(conn: SocialConnection) {
    if (!conn.id || conn.active) return;
    setSocialBusy(conn.provider);
    setSocialError("");
    try {
      const updated = await setActiveSocialConnection(token, conn.provider, conn.id);
      setSocial((prev) =>
        prev.map((p) =>
          p.provider === updated.provider ? { ...p, active: p.id === updated.id, enabled: p.id === updated.id ? updated.enabled : p.enabled } : p,
        ),
      );
    } catch (e) {
      setSocialError((e as Error).message);
    } finally {
      setSocialBusy(null);
    }
  }

  async function disconnectProvider(conn: SocialConnection) {
    if (!conn.id) return;
    const provider = conn.provider;
    setSocialBusy(provider);
    setSocialError("");
    try {
      await disconnectSocialConnection(token, provider, conn.id);
      await refreshSocial();
    } catch (e) {
      setSocialError((e as Error).message);
    } finally {
      setSocialBusy(null);
    }
  }

  return (
    <Portal>
      <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <div
          className="modal note-modal settings-modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="settings-title"
          onClick={(e) => e.stopPropagation()}
        >
          <header className="modal-head">
            <div>
              <p className="eyebrow">Settings</p>
              <h2 id="settings-title">Nimbus settings</h2>
              <p className="meta">Storage API and which Telegram bots Nimbus may use.</p>
            </div>
            <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}>
              <i className="fa-solid fa-xmark" />
            </button>
          </header>
          <div className="note-modal-body stack">
            <section className="settings-section">
              <h3>Storage API</h3>
              {loading && <p className="meta">Loading…</p>}
              {error && <p className="error-text">{error}</p>}
              {!loading && !error && (
                <>
                  <label>
                    API key
                    <div className="row gap" style={{ alignItems: "stretch" }}>
                      <input className="mono-input" readOnly value={apiKey} />
                      <button type="button" className="btn ghost" onClick={() => copy(apiKey, "key")}>
                        {copied === "key" ? "Copied" : "Copy"}
                      </button>
                    </div>
                  </label>
                  <label>
                    API base URL
                    <div className="row gap" style={{ alignItems: "stretch" }}>
                      <input className="mono-input" readOnly value={apiBase} />
                      <button type="button" className="btn ghost" onClick={() => copy(apiBase, "base")}>
                        {copied === "base" ? "Copied" : "Copy"}
                      </button>
                    </div>
                  </label>
                  <p className="meta">
                    Send header <code>X-Nimbus-Key</code> on upload/import. Responses include a public{" "}
                    <code>url</code>.
                  </p>
                </>
              )}
            </section>

            <section className="settings-section">
              <h3>Connected media accounts</h3>
              <p className="meta">
                Click Connect to authorize with your existing browser login. {connectorReady ? "Nimbus Social Connector is ready." : "Install Nimbus Social Connector once for automatic connect."} Switch the active account anytime.
              </p>
              {socialLoading && <p className="meta">Loading connections...</p>}
              {socialError && <p className="error-text">{socialError}</p>}
              {!socialLoading && (
                <ul className="bot-grant-list social-connection-list">
                  {providerOrder.map((provider) => {
                    const accounts = social.filter((conn) => conn.provider === provider && conn.connected);
                    const placeholder = social.find((conn) => conn.provider === provider && !conn.connected);
                    const cookieForm = cookieForms[provider] ?? { displayName: "", file: null };
                    return (
                      <li key={provider} className="social-provider-group">
                        <div className="social-provider-head">
                          <div>
                            <strong>{providerLabel(provider)}</strong>
                            <p className="meta social-empty">
                              {accounts.length ? `${accounts.length} account${accounts.length === 1 ? "" : "s"} connected` : "No account connected"}
                            </p>
                          </div>
                          <button
                            type="button"
                            className="btn ghost compact"
                            disabled={socialBusy === provider}
                            onClick={() => void connectProvider(provider)}
                          >
                            {socialBusy === provider ? "Connecting" : accounts.length ? "Add account" : "Connect"}
                          </button>
                        </div>
                        {accounts.length === 0 && placeholder ? (
                          <p className="meta social-empty">No {providerLabel(provider)} accounts connected.</p>
                        ) : null}
                        <div className="social-cookie-form">
                          <input
                            value={cookieForm.displayName}
                            onChange={(e) => updateCookieForm(provider, { displayName: e.target.value })}
                            placeholder={`${providerLabel(provider)} account label`}
                          />
                          <input
                            type="file"
                            accept=".txt,text/plain"
                            onChange={(e) => updateCookieForm(provider, { file: e.target.files?.[0] ?? null })}
                          />
                          <button
                            type="button"
                            className="btn ghost compact"
                            disabled={cookieBusy === provider}
                            onClick={() => void addCookieAccount(provider)}
                          >
                            {cookieBusy === provider ? "Connecting" : "Connect browser session"}
                          </button>
                        </div>
                        {accounts.map((conn) => (
                          <div key={conn.id} className="bot-grant-row social-connection-row">
                      <div className="bot-grant-meta">
                        <strong>{accountLabel(conn)}</strong>
                        <span className="meta">
                          {conn.active ? (conn.enabled ? "Active for fetches" : "Active but off") : "Connected"}
                          {conn.auth_type ? ` via ${conn.auth_type}` : ""}
                        </span>
                        {conn.scopes?.length ? <span className="meta">{conn.scopes.join(", ")}</span> : null}
                      </div>
                      <div className="row gap social-actions">
                          <>
                            <button
                              type="button"
                              className={`btn ghost compact ${conn.active ? "active" : ""}`}
                              disabled={socialBusy === conn.provider || conn.active}
                              onClick={() => void activateSocial(conn)}
                            >
                              {conn.active ? "Active" : "Switch"}
                            </button>
                            {conn.active ? (
                              <button
                                type="button"
                                className={`bot-toggle ${conn.enabled ? "on" : ""}`}
                                role="switch"
                                aria-checked={conn.enabled}
                                disabled={socialBusy === conn.provider}
                                onClick={() => void toggleSocial(conn)}
                              >
                                <span className="bot-toggle-knob" />
                                <span className="sr-only">{conn.enabled ? "Enabled" : "Disabled"}</span>
                              </button>
                            ) : null}
                            <button
                              type="button"
                              className="btn ghost compact"
                              disabled={socialBusy === conn.provider}
                              onClick={() => void disconnectProvider(conn)}
                            >
                              Disconnect
                            </button>
                          </>
                      </div>
                          </div>
                        ))}
                      </li>
                    );
                  })}
                </ul>
              )}
            </section>

            <section className="settings-section">
              <h3>Save from X / other apps</h3>
              <p className="meta">
                Install Nimbus on your phone (browser menu → <strong>Add to Home Screen</strong>). Then in X,
                TikTok, etc. tap Share and pick <strong>Nimbus</strong> — the link opens Fetch automatically.
                Works best on Android Chrome; iOS support depends on Safari version.
              </p>
            </section>

            <section className="settings-section">
              <h3>Telegram bots</h3>
              <p className="meta">
                Bots you&apos;ve started in Telegram appear here. They stay{" "}
                <strong>blocked</strong> until you allow them — then you can send Drive files to them
                (enhance, compress, convert bots, etc.) just like a Telegram DM.
              </p>
              {botsLoading && <p className="meta">Loading bots from Telegram…</p>}
              {botsError && <p className="error-text">{botsError}</p>}
              {!botsLoading && !botsError && bots.length === 0 && (
                <p className="meta">No bots found. Open a bot in Telegram (e.g. tap Start), then refresh.</p>
              )}
              {!botsLoading && bots.length > 0 && (
                <ul className="bot-grant-list">
                  {bots.map((bot) => (
                    <li key={bot.id} className="bot-grant-row">
                      <div className="bot-grant-meta">
                        <strong>{bot.display_name}</strong>
                        {bot.username ? <span className="meta">@{bot.username}</span> : null}
                      </div>
                      <button
                        type="button"
                        className={`bot-toggle ${bot.allowed ? "on" : ""}`}
                        role="switch"
                        aria-checked={bot.allowed}
                        disabled={busyId === bot.id}
                        onClick={() => void toggleBot(bot)}
                      >
                        <span className="bot-toggle-knob" />
                        <span className="sr-only">{bot.allowed ? "Allowed" : "Blocked"}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              <button
                type="button"
                className="btn ghost compact"
                disabled={botsLoading}
                onClick={() => {
                  setBotsLoading(true);
                  setBotsError("");
                  listBots(token)
                    .then((r) => setBots(r.items ?? []))
                    .catch((e) => setBotsError((e as Error).message))
                    .finally(() => setBotsLoading(false));
                }}
              >
                Refresh bot list
              </button>
            </section>
          </div>
          <footer className="modal-foot row end gap">
            <button type="button" className="btn" onClick={onClose}>
              Done
            </button>
          </footer>
        </div>
      </div>
    </Portal>
  );
}

const providerOrder: SocialProvider[] = ["tiktok", "instagram", "facebook"];

type NimbusConnectorMessage = {
  type: "NIMBUS_SOCIAL_CONNECTOR_READY" | "NIMBUS_SOCIAL_CONNECT_RESULT";
  provider?: SocialProvider;
  ok?: boolean;
  cookies?: string;
  label?: string;
  error?: string;
};

function isSocialProvider(provider: unknown): provider is SocialProvider {
  return provider === "tiktok" || provider === "instagram" || provider === "facebook";
}

function providerLabel(provider: SocialProvider) {
  switch (provider) {
    case "tiktok":
      return "TikTok";
    case "instagram":
      return "Instagram";
    case "facebook":
      return "Facebook";
    default:
      return provider;
  }
}

function providerLoginURL(provider: SocialProvider) {
  switch (provider) {
    case "tiktok":
      return "https://www.tiktok.com/login";
    case "instagram":
      return "https://www.instagram.com/accounts/login/";
    case "facebook":
      return "https://www.facebook.com/login/";
    default:
      return "/";
  }
}

function accountLabel(conn: SocialConnection) {
  if (conn.display_name) return conn.display_name;
  if (conn.username) return `@${conn.username}`;
  if (conn.account_id) return conn.account_id;
  return "Connected account";
}
