import { useEffect, useRef, useState } from "react";
import { Bot, Check, Copy, Loader2 } from "lucide-react";
import {
  addSocialCookieConnection,
  disconnectSocialConnection,
  fetchStorageSettings,
  fetchBackupSettings,
  saveBackupSettings,
  testBackupSettings,
  startBackup,
  fetchS3Settings,
  saveS3Settings,
  listBots,
  listSocialConnections,
  setBotAllowed,
  setActiveSocialConnection,
  setSocialConnectionEnabled,
  type SocialConnection,
  type SocialProvider,
  type TelegramBot,
} from "../api";
import { cn } from "@/lib/utils";
import { copyText } from "../lib/clipboard";
import { PageHeader } from "./PageHeader";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Skeleton } from "./ui/skeleton";
import { Switch } from "./ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./ui/tabs";
import { Textarea } from "./ui/textarea";

type Props = {
  token: string;
};

const providerOrder: SocialProvider[] = ["tiktok", "instagram", "facebook"];

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

export function SettingsPage({ token }: Props) {
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
  const [backupEndpoint, setBackupEndpoint] = useState("");
  const [backupBucket, setBackupBucket] = useState("");
  const [backupRegion, setBackupRegion] = useState("us-east-1");
  const [backupPrefix, setBackupPrefix] = useState("");
  const [backupAccessKeyId, setBackupAccessKeyId] = useState("");
  const [backupSecretKey, setBackupSecretKey] = useState("");
  const [backupEnabled, setBackupEnabled] = useState(false);
  const [backupUseSsl, setBackupUseSsl] = useState(true);
  const [backupPathStyle, setBackupPathStyle] = useState(false);
  const [backupCredentialsConfigured, setBackupCredentialsConfigured] = useState(false);
  const [backupBusy, setBackupBusy] = useState(false);
  const [backupMessage, setBackupMessage] = useState("");
  const [backupError, setBackupError] = useState("");
  const [s3, setS3] = useState<{
    enabled: boolean;
    region: string;
    accessKey: string;
    secretKey: string;
    endpoint: string;
    rclone: string;
  } | null>(null);
  const [s3Region, setS3Region] = useState("us-east-1");
  const [s3Busy, setS3Busy] = useState(false);
  const [s3Message, setS3Message] = useState("");
  const [s3Error, setS3Error] = useState("");
  const [showS3Secret, setShowS3Secret] = useState(false);
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
    fetchBackupSettings(token)
      .then((s) => {
        setBackupEndpoint(s.endpoint);
        setBackupBucket(s.bucket);
        setBackupRegion(s.region || "us-east-1");
        setBackupPrefix(s.prefix || "");
        setBackupAccessKeyId(s.access_key_id_hint || "");
        setBackupEnabled(s.enabled);
        setBackupUseSsl(s.use_ssl);
        setBackupPathStyle(s.path_style);
        setBackupCredentialsConfigured(s.credentials_configured);
      })
      .catch(() => undefined);

    fetchS3Settings(token)
      .then((s) => setS3({
        enabled: s.enabled,
        region: s.region,
        accessKey: s.access_key,
        secretKey: s.secret_key,
        endpoint: s.endpoint,
        rclone: s.rclone,
      }))
      .catch(() => undefined);

    listSocialConnections(token)
      .then((r) => {
        setSocial(r.items ?? []);
        setConnectorReady(true);
      })
      .catch((e) => setSocialError((e as Error).message))
      .finally(() => setSocialLoading(false));

    listBots(token)
      .then((r) => setBots(r.items ?? []))
      .catch((e) => setBotsError((e as Error).message))
      .finally(() => setBotsLoading(false));
  }, [token]);

  async function copy(text: string, label: string) {
    const ok = await copyText(text);
    if (ok) {
      setCopied(label);
      window.setTimeout(() => setCopied(null), 2000);
    } else {
      setError("Copy blocked by the browser — select the text by hand.");
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
    const url = `https://${provider}.com/login`;
    if (!connectorReady) {
      const opened = window.open(url, "_blank", "noopener,noreferrer");
      if (!opened) {
        window.location.href = url;
        return;
      }
      setSocialBusy(null);
      setSocialError("Upload a cookies.txt below to connect this account.");
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

  async function handleSaveBackup() {
    setBackupBusy(true);
    setBackupError("");
    setBackupMessage("");
    try {
      await saveBackupSettings(token, {
        enabled: backupEnabled,
        endpoint: backupEndpoint,
        region: backupRegion,
        bucket: backupBucket,
        prefix: backupPrefix,
        access_key_id: backupAccessKeyId,
        secret_key: backupSecretKey,
        use_ssl: backupUseSsl,
        path_style: backupPathStyle,
      });
      setBackupSecretKey("");
      setBackupCredentialsConfigured(true);
      setBackupMessage("Backup settings saved.");
    } catch (e) {
      setBackupError((e as Error).message);
    } finally {
      setBackupBusy(false);
    }
  }

  async function handleTestBackup() {
    setBackupBusy(true);
    setBackupError("");
    setBackupMessage("");
    try {
      const result = await testBackupSettings(token);
      setBackupMessage(result.message || "Connection successful");
    } catch (e) {
      setBackupError((e as Error).message);
    } finally {
      setBackupBusy(false);
    }
  }

  async function handleRunBackup() {
    setBackupBusy(true);
    setBackupError("");
    setBackupMessage("");
    try {
      const result = await startBackup(token, "database");
      setBackupMessage(`Backup started: ${result.job_id}`);
    } catch (e) {
      setBackupError((e as Error).message);
    } finally {
      setBackupBusy(false);
    }
  }

  function applyS3Settings(s: {
    enabled: boolean;
    region: string;
    access_key: string;
    secret_key: string;
    endpoint: string;
    rclone: string;
  }) {
    setS3({
      enabled: s.enabled,
      region: s.region,
      accessKey: s.access_key,
      secretKey: s.secret_key,
      endpoint: s.endpoint,
      rclone: s.rclone,
    });
    setS3Region(s.region);
  }

  async function handleSaveS3Region() {
    setS3Busy(true);
    setS3Error("");
    setS3Message("");
    try {
      const s = await saveS3Settings(token, { region: s3Region });
      applyS3Settings(s);
      setS3Message("Region saved. Restart Nimbus to apply.");
    } catch (e) {
      setS3Error((e as Error).message);
    } finally {
      setS3Busy(false);
    }
  }

  async function handleRotateS3Keys() {
    setS3Busy(true);
    setS3Error("");
    setS3Message("");
    try {
      const s = await saveS3Settings(token, { regenerate: true });
      applyS3Settings(s);
      setS3Message("Keys regenerated. Reconfigure your S3 clients.");
    } catch (e) {
      setS3Error((e as Error).message);
    } finally {
      setS3Busy(false);
    }
  }

  async function copyS3Rclone() {
    if (!s3) return;
    const ok = await copyText(s3.rclone);
    if (ok) setS3Message("rclone config copied to clipboard.");
    else setS3Error("Copy blocked by the browser. Select the text manually.");
  }

  function updateCookieForm(provider: SocialProvider, patch: Partial<{ displayName: string; file: File | null }>) {
    setCookieForms((prev) => {
      const nextForm = prev[provider] ?? { displayName: "", file: null };
      return { ...prev, [provider]: { ...nextForm, ...patch } };
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
    <div className="grid gap-5">
      <PageHeader title="Settings" description="Storage API, backups, connected accounts and Telegram bots." />

      <Tabs defaultValue="storage" className="w-full">
        <TabsList className="grid w-full max-w-xl grid-cols-4">
          <TabsTrigger value="storage">Storage</TabsTrigger>
          <TabsTrigger value="backup">Backup</TabsTrigger>
          <TabsTrigger value="social">Accounts</TabsTrigger>
          <TabsTrigger value="bots">Bots</TabsTrigger>
        </TabsList>

        <TabsContent value="storage" className="grid max-w-3xl gap-4">
          {loading ? (
            <div className="grid gap-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : (
            <>
              <Card>
                <CardHeader>
                  <CardTitle>API access</CardTitle>
                  <CardDescription>Use these to upload/import through the REST storage API.</CardDescription>
                </CardHeader>
                <CardContent className="grid gap-4">
                  <div className="grid gap-1.5">
                    <Label htmlFor="s-api-key">API key</Label>
                    <div className="flex gap-2">
                      <Input id="s-api-key" readOnly value={apiKey} className="font-mono text-xs" />
                      <Button variant="outline" onClick={() => copy(apiKey, "key")}>
                        {copied === "key" ? <Check /> : <Copy />}
                        {copied === "key" ? "Copied" : "Copy"}
                      </Button>
                    </div>
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor="s-api-base">API base URL</Label>
                    <div className="flex gap-2">
                      <Input id="s-api-base" readOnly value={apiBase} className="font-mono text-xs" />
                      <Button variant="outline" onClick={() => copy(apiBase, "base")}>
                        {copied === "base" ? <Check /> : <Copy />}
                        {copied === "base" ? "Copied" : "Copy"}
                      </Button>
                    </div>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Send header <code className="rounded bg-muted px-1">X-Nimbus-Key</code> on upload/import.
                  </p>
                </CardContent>
              </Card>

              {!s3 ? (
                <p className="text-sm text-muted-foreground">Loading S3 access…</p>
              ) : (
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      S3-compatible access
                      {!s3.enabled && <Badge variant="muted">Disabled</Badge>}
                    </CardTitle>
                    <CardDescription>Point rclone, aws-cli or any S3 client at this endpoint.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-3">
                    {s3Error && (
                      <Alert variant="destructive">
                        <AlertDescription>{s3Error}</AlertDescription>
                      </Alert>
                    )}
                    {s3Message && (
                      <Alert>
                        <AlertDescription>{s3Message}</AlertDescription>
                      </Alert>
                    )}
                    <div className="grid gap-1.5">
                      <Label>Endpoint</Label>
                      <Input value={s3.endpoint} readOnly className="font-mono text-xs" />
                    </div>
                    <div className="grid gap-3 sm:grid-cols-2">
                      <div className="grid gap-1.5">
                        <Label htmlFor="s3-region">Region</Label>
                        <Input id="s3-region" value={s3Region} onChange={(e) => setS3Region(e.target.value)} />
                      </div>
                      <div className="grid gap-1.5">
                        <Label>Access Key ID</Label>
                        <Input value={s3.accessKey} readOnly className="font-mono text-xs" />
                      </div>
                    </div>
                    <div className="grid gap-1.5">
                      <Label>Secret Access Key</Label>
                      <Input type={showS3Secret ? "text" : "password"} value={s3.secretKey} readOnly className="font-mono text-xs" />
                    </div>
                    <label className="flex cursor-pointer items-center gap-2 text-sm">
                      <Switch checked={showS3Secret} onCheckedChange={setShowS3Secret} />
                      Show secret
                    </label>
                    <div className="grid gap-1.5">
                      <Label>rclone config</Label>
                      <Textarea rows={6} value={s3.rclone} readOnly className="font-mono text-xs" />
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button disabled={s3Busy} onClick={() => void handleSaveS3Region()}>Save region</Button>
                      <Button variant="outline" disabled={s3Busy} onClick={() => void copyS3Rclone()}>Copy rclone config</Button>
                      <Button variant="outline" disabled={s3Busy} onClick={() => void handleRotateS3Keys()}>Regenerate keys</Button>
                    </div>
                  </CardContent>
                </Card>
              )}
            </>
          )}
        </TabsContent>

        <TabsContent value="backup" className="grid max-w-3xl gap-3">
          <Card>
            <CardHeader>
              <CardTitle>Database backup</CardTitle>
              <CardDescription>S3-compatible storage for automatic database backups.</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3">
              {backupError && (
                <Alert variant="destructive">
                  <AlertDescription>{backupError}</AlertDescription>
                </Alert>
              )}
              {backupMessage && (
                <Alert>
                  <Check className="h-4 w-4 text-green-600" />
                  <AlertDescription>{backupMessage}</AlertDescription>
                </Alert>
              )}
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label htmlFor="b-endpoint">Endpoint</Label>
                  <Input id="b-endpoint" value={backupEndpoint} onChange={(e) => setBackupEndpoint(e.target.value)} placeholder="s3.amazonaws.com" />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="b-bucket">Bucket</Label>
                  <Input id="b-bucket" value={backupBucket} onChange={(e) => setBackupBucket(e.target.value)} />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="b-region">Region</Label>
                  <Input id="b-region" value={backupRegion} onChange={(e) => setBackupRegion(e.target.value)} />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="b-prefix">Prefix</Label>
                  <Input id="b-prefix" value={backupPrefix} onChange={(e) => setBackupPrefix(e.target.value)} placeholder="nimbus-backups/" />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="b-key">Access Key ID</Label>
                  <Input id="b-key" value={backupAccessKeyId} onChange={(e) => setBackupAccessKeyId(e.target.value)} />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="b-secret">Secret Key</Label>
                  <Input id="b-secret" type="password" autoComplete="new-password" placeholder={backupCredentialsConfigured ? "Unchanged" : "Required"} value={backupSecretKey} onChange={(e) => setBackupSecretKey(e.target.value)} />
                </div>
              </div>
              <div className="flex flex-wrap gap-4">
                <label className="flex cursor-pointer items-center gap-2 text-sm">
                  <Switch checked={backupEnabled} onCheckedChange={setBackupEnabled} /> Enabled
                </label>
                <label className="flex cursor-pointer items-center gap-2 text-sm">
                  <Switch checked={backupUseSsl} onCheckedChange={setBackupUseSsl} /> Use SSL
                </label>
                <label className="flex cursor-pointer items-center gap-2 text-sm">
                  <Switch checked={backupPathStyle} onCheckedChange={setBackupPathStyle} /> Path-style
                </label>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button disabled={backupBusy} onClick={() => void handleSaveBackup()}>
                  {backupBusy && <Loader2 className="h-4 w-4 animate-spin" />} Save
                </Button>
                <Button variant="outline" disabled={backupBusy || (!backupCredentialsConfigured && !backupSecretKey)} onClick={() => void handleTestBackup()}>
                  Test connection
                </Button>
                <Button variant="outline" disabled={backupBusy || !backupEnabled} onClick={() => void handleRunBackup()}>
                  Run backup now
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="social" className="grid max-w-3xl gap-3">
          <p className="text-sm text-muted-foreground">
            Connect with your browser login. {connectorReady ? "Connector is ready." : "Install the Nimbus Social Connector once for one-click connect."}
          </p>
          {socialError && (
            <Alert variant="destructive">
              <AlertDescription>{socialError}</AlertDescription>
            </Alert>
          )}
          {socialLoading ? (
            <div className="grid gap-2">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-20 w-full" />
              ))}
            </div>
          ) : (
            <div className="grid gap-3">
              {providerOrder.map((provider) => {
                const accounts = social.filter((conn) => conn.provider === provider && conn.connected);
                const cookieForm = cookieForms[provider] ?? { displayName: "", file: null };
                return (
                  <Card key={provider}>
                    <CardHeader className="flex-row items-center justify-between space-y-0">
                      <div>
                        <CardTitle className="text-base">{providerLabel(provider)}</CardTitle>
                        <CardDescription>{accounts.length ? `${accounts.length} connected` : "No account connected"}</CardDescription>
                      </div>
                      <Button variant="outline" size="sm" disabled={socialBusy === provider} onClick={() => void connectProvider(provider)}>
                        {socialBusy === provider ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
                        {socialBusy === provider ? "Connecting" : accounts.length ? "Add account" : "Connect"}
                      </Button>
                    </CardHeader>
                    <CardContent className="grid gap-2">
                      <div className="grid gap-2 rounded-xl bg-muted/50 p-2.5 sm:grid-cols-[1fr_1fr_auto]">
                        <Input
                          value={cookieForm.displayName}
                          onChange={(e) => updateCookieForm(provider, { displayName: e.target.value })}
                          placeholder={`${providerLabel(provider)} label`}
                        />
                        <Input
                          type="file"
                          accept=".txt,text/plain"
                          onChange={(e) => updateCookieForm(provider, { file: e.target.files?.[0] ?? null })}
                        />
                        <Button variant="outline" size="sm" disabled={cookieBusy === provider} onClick={() => void addCookieAccount(provider)}>
                          {cookieBusy === provider ? "Connecting" : "Upload cookies"}
                        </Button>
                      </div>
                      {accounts.map((conn) => (
                        <div key={conn.id} className="flex items-center justify-between gap-2 rounded-xl bg-muted/50 px-3 py-2.5">
                          <span className="flex min-w-0 items-center gap-2">
                            <span className="truncate text-sm">{conn.display_name || providerLabel(provider)}</span>
                            {conn.active && <Badge variant="secondary">Active</Badge>}
                          </span>
                          <span className="flex items-center gap-2">
                            <Button variant="outline" size="sm" disabled={socialBusy === provider || conn.active} onClick={() => void activateSocial(conn)}>
                              Use
                            </Button>
                            <Switch checked={conn.enabled} disabled={socialBusy === provider} onCheckedChange={() => void toggleSocial(conn)} />
                            <Button variant="ghost" size="sm" disabled={socialBusy === provider} className="text-destructive hover:text-destructive" onClick={() => void disconnectProvider(conn)}>
                              Remove
                            </Button>
                          </span>
                        </div>
                      ))}
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )}
        </TabsContent>

        <TabsContent value="bots" className="grid max-w-3xl gap-3">
          <p className="text-sm text-muted-foreground">Telegram bots allowed to store files in this drive.</p>
          {botsError && (
            <Alert variant="destructive">
              <AlertDescription>{botsError}</AlertDescription>
            </Alert>
          )}
          {botsLoading ? (
            <div className="grid gap-2">
              {[0, 1].map((i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : bots.length === 0 ? (
            <p className="rounded-xl bg-muted/60 px-3.5 py-3 text-sm text-muted-foreground">
              No bots connected yet.
            </p>
          ) : (
            <div className="grid gap-2">
              {bots.map((bot) => (
                <div key={bot.id} className="flex items-center justify-between gap-2 rounded-xl border px-3.5 py-3">
                  <span className="flex min-w-0 items-center gap-2.5">
                    <span className={cn("grid h-9 w-9 place-items-center rounded-full", bot.allowed ? "bg-green-100 text-green-700" : "bg-muted text-muted-foreground")}>
                      <Bot className="h-4 w-4" />
                    </span>
                    <span className="grid min-w-0">
                      <strong className="truncate text-sm">{bot.display_name}</strong>
                      {bot.username && <small className="truncate text-xs text-muted-foreground">@{bot.username}</small>}
                    </span>
                  </span>
                  <Switch checked={bot.allowed} disabled={busyId === bot.id} onCheckedChange={() => void toggleBot(bot)} />
                </div>
              ))}
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
