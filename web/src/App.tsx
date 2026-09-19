import { useCallback, useEffect, useMemo, useState } from "react";
import {
  clearAccessKey,
  createShare,
  deleteNode,
  downloadUrl,
  emptyTrash,
  fetchFromURL,
  fetchJobStatus,
  importFileURL,
  getAccessKey,
  listFiles,
  listShares,
  listTrash,
  logout,
  me,
  moveNodes,
  mkdir,
  purgeNode,
  renameNode,
  resumeSession,
  revokeShare,
  sendCode,
  sendToTelegram,
  setAccessKey,
  setupStatus,
  setupTelegram,
  signIn,
  type Node,
  type ShareInfo,
  type User,
  uploadFile,
  validateAccessKey,
} from "./api";
import { AccessKeyScreen, BootScreen, LoginScreen, SetupScreen } from "./components/AuthScreens";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { DriveShell } from "./components/DriveShell";
import { PromptDialog } from "./components/PromptDialog";
import { ShareModal } from "./components/ShareModal";
import { SendTelegramModal } from "./components/SendTelegramModal";
import { Toaster } from "./components/ui/sonner";
import { toast } from "sonner";
import { UploadPanel, type UploadJob } from "./components/UploadPanel";
import { extractSharedURL, isShareTargetPath } from "./lib/shareTarget";

const TOKEN_KEY = "nimbus_token";

type ToastKind = "success" | "error" | "info";

type ConfirmState = {
  title: string;
  message: string;
  confirmLabel: string;
  danger?: boolean;
  onConfirm: () => void;
};

export default function App() {
  const [accessKeyRequired, setAccessKeyRequired] = useState(true);
  const [accessReady, setAccessReady] = useState(false);
  const [accessDraft, setAccessDraft] = useState("");
  const [token, setToken] = useState<string | null>(() => localStorage.getItem(TOKEN_KEY));
  const [user, setUser] = useState<User | null>(null);
  const [configured, setConfigured] = useState<boolean | null>(null);
  const [parentId, setParentId] = useState("root");
  const [trail, setTrail] = useState<{ id: string; name: string }[]>([
    { id: "root", name: "My Drive" },
  ]);
  const [items, setItems] = useState<Node[]>([]);
  const [trashItems, setTrashItems] = useState<Node[]>([]);
  const [section, setSection] = useState<"drive" | "trash" | "settings" | "buckets" | "dashboard">("dashboard");
  const [shareTarget, setShareTarget] = useState<{ id: string; name: string } | null>(null);
  const [sendTarget, setSendTarget] = useState<{ id: string; name: string } | null>(null);
  const [shares, setShares] = useState<ShareInfo[]>([]);
  const [shareBusy, setShareBusy] = useState(false);
  const [sendBusy, setSendBusy] = useState(false);
  const [sendError, setSendError] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [uploads, setUploads] = useState<UploadJob[]>([]);


  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  const [mkdirOpen, setMkdirOpen] = useState(false);
  const [renameTarget, setRenameTarget] = useState<{ id: string; name: string } | null>(null);
  const [sharedFetchUrl, setSharedFetchUrl] = useState<string | null>(null);

  useEffect(() => {
    const path = window.location.pathname;
    const shared = extractSharedURL(window.location.search);
    if (!shared) return;
    setSharedFetchUrl(shared);
    if (isShareTargetPath(path) || window.location.search.includes("url=") || window.location.search.includes("text=")) {
      window.history.replaceState({}, "", "/");
    }
  }, []);

  function pushToast(message: string, kind: ToastKind = "info") {
    if (kind === "success") toast.success(message);
    else if (kind === "error") toast.error(message);
    else toast(message);
  }

  function askConfirm(state: ConfirmState) {
    setConfirm(state);
  }

  const [apiId, setApiId] = useState("");
  const [apiHash, setApiHash] = useState("");
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [phoneCodeHash, setPhoneCodeHash] = useState("");
  const [need2fa, setNeed2fa] = useState(false);

  const title = useMemo(() => trail[trail.length - 1]?.name ?? "My Drive", [trail]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const params = new URLSearchParams(window.location.search);
      const fromUrl = params.get("access");
      if (fromUrl?.trim()) {
        setAccessKey(fromUrl.trim());
        params.delete("access");
        const qs = params.toString();
        window.history.replaceState(
          {},
          "",
          `${window.location.pathname}${qs ? `?${qs}` : ""}${window.location.hash}`,
        );
      }

      try {
        const s = await setupStatus();
        if (cancelled) return;
        const required = s.access_key_required !== false;
        setAccessKeyRequired(required);
        if (!required) {
          setAccessReady(true);
          return;
        }
        // Trust a stored key only after the server confirms it, otherwise a
        // stale key silently gates every later request behind a 401.
        const stored = getAccessKey();
        if (!stored) return;
        try {
          await validateAccessKey(stored);
          if (cancelled) return;
          setAccessReady(true);
        } catch (e) {
          if (cancelled) return;
          if (isAccessKeyError((e as Error).message)) clearAccessKey();
        }
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!accessReady) return;
    let cancelled = false;
    (async () => {
      try {
        const s = await setupStatus();
        if (cancelled) return;
        setConfigured(s.configured);
        if (!s.configured) return;

        const existing = localStorage.getItem(TOKEN_KEY);
        if (existing) {
          try {
            const u = await me(existing);
            if (cancelled) return;
            setToken(existing);
            setUser(u);
            return;
          } catch {
            localStorage.removeItem(TOKEN_KEY);
          }
        }

        if (s.authorized) {
          const r = await resumeSession();
          if (cancelled) return;
          localStorage.setItem(TOKEN_KEY, r.token);
          setToken(r.token);
          setUser(r.user);
        }
      } catch (e) {
        if (!cancelled) {
          const msg = (e as Error).message;
          if (msg.toLowerCase().includes("access key")) {
            clearAccessKey();
            setAccessReady(false);
            setError(msg);
            return;
          }
          setConfigured(false);
          setError(msg);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [accessReady]);

  useEffect(() => {
    if (!token) return;
    setBusy(true);
    setError("");
    listFiles(token, parentId)
      .then((r) => setItems(r.items ?? []))
      .catch((e: Error) => setError(e.message))
      .finally(() => setBusy(false));
  }, [token, parentId]);

  async function refreshTrash() {
    if (!token) return;
    try {
      const r = await listTrash(token);
      setTrashItems(r.items ?? []);
    } catch {
      setTrashItems([]);
    }
  }

  useEffect(() => {
    if (!token) return;
    void refreshTrash();
  }, [token]);

  async function onUnlockAccess() {
    setError("");
    setBusy(true);
    try {
      await validateAccessKey(accessDraft);
      setAccessKey(accessDraft);
      setAccessReady(true);
      setAccessDraft("");
    } catch (e) {
      clearAccessKey();
      setAccessReady(false);
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  function bounceToAccessScreen(msg: string) {
    clearAccessKey();
    setAccessReady(false);
    setError(msg);
  }

  function isAccessKeyError(msg: string) {
    return msg.toLowerCase().includes("access key");
  }

  async function onSaveApi() {
    setError("");
    setBusy(true);
    try {
      const id = Number(apiId.trim());
      if (!Number.isFinite(id) || id <= 0) throw new Error("API ID must be a number");
      await setupTelegram(id, apiHash.trim());
      setConfigured(true);
    } catch (e) {
      const msg = (e as Error).message;
      if (isAccessKeyError(msg)) {
        bounceToAccessScreen(msg);
        return;
      }
      setError(msg);
    } finally {
      setBusy(false);
    }
  }

  async function onSendCode() {
    setError("");
    setBusy(true);
    try {
      const r = await sendCode(phone);
      setPhoneCodeHash(r.phone_code_hash);
    } catch (e) {
      const msg = (e as Error).message;
      if (isAccessKeyError(msg)) {
        bounceToAccessScreen(msg);
        return;
      }
      setError(msg);
    } finally {
      setBusy(false);
    }
  }

  async function onSignIn() {
    setError("");
    setBusy(true);
    try {
      const r = await signIn({
        phone,
        code,
        phone_code_hash: phoneCodeHash,
        password: need2fa ? password : undefined,
      });
      localStorage.setItem(TOKEN_KEY, r.token);
      setToken(r.token);
      setUser(r.user);
    } catch (e) {
      const msg = (e as Error).message;
      if (isAccessKeyError(msg)) {
        bounceToAccessScreen(msg);
        return;
      }
      if (msg.includes("two-factor") || msg.includes("two_fa")) setNeed2fa(true);
      setError(msg);
    } finally {
      setBusy(false);
    }
  }

  async function onLogout() {
    if (!token) return;
    try {
      await logout(token);
    } catch {
      /* ignore */
    }
    localStorage.removeItem(TOKEN_KEY);
    setToken(null);
    setUser(null);
  }

  function openFolder(n: Node) {
    setParentId(n.id);
    setTrail((t) => [...t, { id: n.id, name: n.name }]);
  }

  function navigate(crumbs: { id: string; name: string }[]) {
    if (!crumbs.length) return;
    setTrail(crumbs);
    setParentId(crumbs[crumbs.length - 1].id);
  }

  function goCrumb(index: number) {
    const next = trail.slice(0, index + 1);
    setTrail(next);
    setParentId(next[next.length - 1].id);
  }

  async function onMkdir() {
    if (!token) return;
    setMkdirOpen(true);
  }

  async function doMkdir(name: string) {
    if (!token) return;
    setMkdirOpen(false);
    setBusy(true);
    try {
      await mkdir(token, parentId, name.trim());
      const r = await listFiles(token, parentId);
      setItems(r.items ?? []);
      pushToast(`Created “${name.trim()}”`, "success");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function onFetchURL(
    input: {
      urls: string[];
      mode: "video" | "audio";
      maxHeight: number | null;
    },
    onProgress?: (p: { progress: number; message: string }) => void,
  ) {
    if (!token) return;
    setError("");
    const list = input.urls;
    let saved = 0;
    for (let i = 0; i < list.length; i++) {
      const url = list[i];
      const base = list.length > 1 ? (i / list.length) * 100 : 0;
      const span = list.length > 1 ? 100 / list.length : 100;
      onProgress?.({
        progress: base,
        message: list.length > 1 ? `Link ${i + 1}/${list.length}…` : "Starting…",
      });
      const started = await fetchFromURL(token, parentId, {
        url,
        mode: input.mode,
        max_height: input.maxHeight,
      });
      for (;;) {
        await new Promise((r) => setTimeout(r, 400));
        const st = await fetchJobStatus(token, started.job_id);
        const local = st.progress ?? 0;
        onProgress?.({
          progress: Math.min(99, base + (local / 100) * span),
          message:
            list.length > 1
              ? `Link ${i + 1}/${list.length}: ${st.message || ""}`
              : st.message || "",
        });
        if (st.status === "done") {
          saved += st.count || st.nodes?.length || (st.node ? 1 : 0);
          break;
        }
        if (st.status === "error") {
          throw new Error(
            list.length > 1
              ? `Link ${i + 1}/${list.length}: ${st.message || "Fetch failed"}`
              : st.message || "Fetch failed",
          );
        }
      }
    }
    const r = await listFiles(token, parentId);
    setItems(r.items ?? []);
    onProgress?.({ progress: 100, message: "Done" });
    if (list.length > 1) pushToast(`Fetched ${list.length} links (${saved} files)`, "success");
    else if (saved > 1) pushToast(`Saved ${saved} files to Drive`, "success");
    else pushToast("Saved to Drive", "success");
  }

  async function onImportURL(urls: string[]) {
    if (!token) return;
    setError("");
    setBusy(true);
    try {
      let lastName = "";
      for (const url of urls) {
        const file = await importFileURL(token, parentId, url);
        lastName = file.name;
        if (file.duplicate) {
          pushToast(`${file.name} already in Drive`, "info");
        }
      }
      const r = await listFiles(token, parentId);
      setItems(r.items ?? []);
      if (urls.length > 1) pushToast(`Imported ${urls.length} files`, "success");
      else if (lastName) pushToast(`Imported ${lastName}`, "success");
    } catch (e) {
      setError((e as Error).message);
      throw e;
    } finally {
      setBusy(false);
    }
  }

  function patchUpload(id: string, patch: Partial<UploadJob>) {
    setUploads((prev) => prev.map((j) => (j.id === id ? { ...j, ...patch } : j)));
  }

  const clearUploads = useCallback(() => setUploads([]), []);
  const dismissUpload = useCallback(
    (id: string) => setUploads((prev) => prev.filter((j) => j.id !== id)),
    [],
  );

  async function onUpload(files: FileList | File[]) {
    if (!token) return;
    const list = Array.from(files);
    if (!list.length) return;

    const jobs: UploadJob[] = list.map((file) => ({
      id: `${Date.now()}-${file.name}-${Math.random().toString(36).slice(2, 8)}`,
      name: file.name,
      size: file.size,
      progress: 0,
      status: "queued",
    }));
    setUploads((prev) => [...jobs, ...prev].slice(0, 20));
    setError("");

    const concurrency = Math.min(3, list.length);
    let cursor = 0;

    async function worker() {
      while (cursor < list.length) {
        const i = cursor++;
        const file = list[i];
        const job = jobs[i];
        patchUpload(job.id, { status: "uploading", progress: 0 });
        try {
          const node = await uploadFile(
            token!,
            parentId,
            file,
            (pct) => patchUpload(job.id, { progress: pct, status: "uploading" }),
            () => patchUpload(job.id, { progress: 100, status: "processing" }),
          );
          patchUpload(job.id, { status: "done", progress: 100 });
          if (node.duplicate) pushToast(`${file.name} already in Drive`, "info");
          else pushToast(`Uploaded ${file.name}`, "success");
          const r = await listFiles(token!, parentId);
          setItems(r.items ?? []);
        } catch (e) {
          patchUpload(job.id, { status: "error", error: (e as Error).message });
          pushToast((e as Error).message, "error");
        }
      }
    }

    await Promise.all(Array.from({ length: concurrency }, () => worker()));
  }

  async function onRename(id: string, currentName: string) {
    if (!token) return;
    setRenameTarget({ id, name: currentName });
  }

  async function doRename(name: string) {
    if (!token || !renameTarget) return;
    const { id, name: currentName } = renameTarget;
    if (!name || name === currentName) {
      setRenameTarget(null);
      return;
    }
    setRenameTarget(null);
    setBusy(true);
    try {
      await renameNode(token, id, name);
      const r = await listFiles(token, parentId);
      setItems(r.items ?? []);
      setTrail((t) => t.map((c) => (c.id === id ? { ...c, name } : c)));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function onMoveMany(ids: string[], destParentId: string) {
    if (!token || !ids.length) return;
    setBusy(true);
    setError("");
    try {
      await moveNodes(token, ids, destParentId);
      const r = await listFiles(token, parentId);
      setItems(r.items ?? []);
      pushToast(`Moved ${ids.length} item${ids.length === 1 ? "" : "s"}`, "success");
    } catch (e) {
      setError((e as Error).message);
      pushToast((e as Error).message, "error");
      throw e;
    } finally {
      setBusy(false);
    }
  }

  function onDelete(id: string) {
    if (!token) return;
    askConfirm({
      title: "Move to trash?",
      message: "This item will stay in Trash until you delete it permanently.",
      confirmLabel: "Move to trash",
      danger: true,
      onConfirm: () => void doDelete(id),
    });
  }

  async function doDelete(id: string) {
    if (!token) return;
    setConfirm(null);
    setBusy(true);
    try {
      await deleteNode(token, id);
      const r = await listFiles(token, parentId);
      setItems(r.items ?? []);
      await refreshTrash();
      pushToast("Moved to trash", "success");
    } catch (e) {
      setError((e as Error).message);
      pushToast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  function onDeleteMany(ids: string[]) {
    if (!token || !ids.length) return;
    askConfirm({
      title: `Move ${ids.length} item${ids.length === 1 ? "" : "s"} to trash?`,
      message: "These items will stay in Trash until you delete them permanently.",
      confirmLabel: "Move to trash",
      danger: true,
      onConfirm: () => void doDeleteMany(ids),
    });
  }

  async function doDeleteMany(ids: string[]) {
    if (!token) return;
    setConfirm(null);
    setBusy(true);
    setError("");
    try {
      for (const id of ids) {
        await deleteNode(token, id);
      }
      const r = await listFiles(token, parentId);
      setItems(r.items ?? []);
      await refreshTrash();
      pushToast(`Moved ${ids.length} item${ids.length === 1 ? "" : "s"} to trash`, "success");
    } catch (e) {
      setError((e as Error).message);
      pushToast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  function onPurge(id: string) {
    if (!token) return;
    askConfirm({
      title: "Delete forever?",
      message: "This cannot be undone.",
      confirmLabel: "Delete forever",
      danger: true,
      onConfirm: () => void doPurge(id),
    });
  }

  async function doPurge(id: string) {
    if (!token) return;
    setConfirm(null);
    setBusy(true);
    try {
      await purgeNode(token, id);
      await refreshTrash();
      pushToast("Deleted permanently", "info");
    } catch (e) {
      setError((e as Error).message);
      pushToast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  function onEmptyTrash() {
    if (!token) return;
    askConfirm({
      title: "Empty trash?",
      message: "All items will be deleted forever.",
      confirmLabel: "Empty trash",
      danger: true,
      onConfirm: () => void doEmptyTrash(),
    });
  }

  async function doEmptyTrash() {
    if (!token) return;
    setConfirm(null);
    setBusy(true);
    try {
      await emptyTrash(token);
      await refreshTrash();
      pushToast("Trash emptied", "info");
    } catch (e) {
      setError((e as Error).message);
      pushToast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function openShare(id: string, name: string) {
    if (!token) return;
    setShareTarget({ id, name });
    setShareBusy(true);
    try {
      const r = await listShares(token, id);
      setShares(r.items ?? []);
    } catch (e) {
      setError((e as Error).message);
      setShares([]);
    } finally {
      setShareBusy(false);
    }
  }

  async function onCreateShare(): Promise<ShareInfo | null> {
    if (!token || !shareTarget) return null;
    setShareBusy(true);
    try {
      const created = await createShare(token, shareTarget.id);
      const r = await listShares(token, shareTarget.id);
      setShares(r.items ?? []);
      return created;
    } catch (e) {
      setError((e as Error).message);
      pushToast((e as Error).message, "error");
      return null;
    } finally {
      setShareBusy(false);
    }
  }

  function onSectionChange(next: "drive" | "trash" | "settings" | "buckets" | "dashboard") {
    setSection(next);
    if (next === "trash") void refreshTrash();
  }

  async function onRevokeShare(shareId: string) {
    if (!token || !shareTarget) return;
    setShareBusy(true);
    try {
      await revokeShare(token, shareId);
      const r = await listShares(token, shareTarget.id);
      setShares(r.items ?? []);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setShareBusy(false);
    }
  }

  function openSendTelegram(id: string, name: string) {
    setSendError("");
    setSendTarget({ id, name });
  }

  async function onSendTelegram(contact: { id: number; display_name: string }) {
    if (!token || !sendTarget) return;
    setSendBusy(true);
    setSendError("");
    try {
      await sendToTelegram(token, sendTarget.id, contact.id);
      setSendTarget(null);
      pushToast(`Sent "${sendTarget.name}" to ${contact.display_name}`, "success");
    } catch (e) {
      const message = (e as Error).message;
      setSendError(message);
      pushToast(message, "error");
    } finally {
      setSendBusy(false);
    }
  }

  function onDownload(id: string) {
    if (!token) return;
    const headers: HeadersInit = { Authorization: `Bearer ${token}` };
    const access = getAccessKey();
    if (access) (headers as Record<string, string>)["X-Nimbus-Access"] = access;
    fetch(downloadUrl(id), { headers })
      .then(async (res) => {
        if (!res.ok) throw new Error("Download failed");
        const blob = await res.blob();
        const a = document.createElement("a");
        a.href = URL.createObjectURL(blob);
        a.download = items.find((i) => i.id === id)?.name ?? "download";
        a.click();
        URL.revokeObjectURL(a.href);
      })
      .catch((e: Error) => setError(e.message));
  }

  if (!accessReady) {
    return (
      <AccessKeyScreen
        value={accessDraft}
        busy={busy}
        error={error}
        onChange={setAccessDraft}
        onSubmit={onUnlockAccess}
        showAdvanced={accessKeyRequired}
      />
    );
  }

  if (configured === null) return <BootScreen />;

  if (!configured) {
    return (
      <SetupScreen
        apiId={apiId}
        apiHash={apiHash}
        busy={busy}
        error={error}
        onApiId={setApiId}
        onApiHash={setApiHash}
        onSave={onSaveApi}
      />
    );
  }

  if (!token) {
    return (
      <LoginScreen
        phone={phone}
        code={code}
        password={password}
        phoneCodeHash={phoneCodeHash}
        need2fa={need2fa}
        busy={busy}
        error={error}
        onPhone={setPhone}
        onCode={setCode}
        onPassword={setPassword}
        onSendCode={onSendCode}
        onSignIn={onSignIn}
        onChangeNumber={() => {
          setPhoneCodeHash("");
          setCode("");
          setPassword("");
          setNeed2fa(false);
          setError("");
        }}
      />
    );
  }

  return (
    <>
      <DriveShell
        token={token}
        user={user}
        section={section}
        title={title}
        trail={trail}
        items={items}
        trashItems={trashItems}
        busy={busy}
        error={error}
        onSectionChange={onSectionChange}
        onCrumb={goCrumb}
        onOpenFolder={openFolder}
        onNavigate={navigate}
        onMkdir={onMkdir}
        onFetchURL={onFetchURL}
        onImportURL={onImportURL}
        onUpload={onUpload}
        onDownload={onDownload}
        onRename={onRename}
        onMoveMany={onMoveMany}
        onDelete={onDelete}
        onDeleteMany={onDeleteMany}
        onPurge={onPurge}
        onEmptyTrash={onEmptyTrash}
        onShare={openShare}
        onSendTelegram={openSendTelegram}
        onRefresh={async () => {
          if (!token) return;
          try {
            const r = await listFiles(token, parentId);
            setItems(r.items ?? []);
          } catch (e) {
            setError((e as Error).message);
          }
        }}
        onLogout={onLogout}
        onClearError={() => setError("")}
        sharedFetchUrl={sharedFetchUrl}
        onSharedFetchConsumed={() => setSharedFetchUrl(null)}
      />
      {sendTarget && (
        <SendTelegramModal
          token={token}
          fileName={sendTarget.name}
          busy={sendBusy}
          error={sendError}
          onClose={() => {
            setSendTarget(null);
            setSendError("");
          }}
          onSend={onSendTelegram}
        />
      )}
      {shareTarget && (
        <ShareModal
          fileName={shareTarget.name}
          busy={shareBusy}
          shares={shares}
          onClose={() => {
            setShareTarget(null);
            setShares([]);
          }}
          onCreate={onCreateShare}
          onRevoke={onRevokeShare}
        />
      )}
      {confirm && (
        <ConfirmDialog
          title={confirm.title}
          message={confirm.message}
          confirmLabel={confirm.confirmLabel}
          danger={confirm.danger}
          busy={busy}
          onCancel={() => setConfirm(null)}
          onConfirm={confirm.onConfirm}
        />
      )}
      <PromptDialog
        open={mkdirOpen}
        title="New folder"
        description="Give your new folder a cozy name."
        label="Folder name"
        placeholder="e.g. Vacation photos"
        confirmLabel="Create folder"
        busy={busy}
        onClose={() => setMkdirOpen(false)}
        onConfirm={(name) => void doMkdir(name)}
      />
      <PromptDialog
        open={renameTarget !== null}
        title="Rename"
        description={renameTarget ? `Rename “${renameTarget.name}”.` : undefined}
        label="Name"
        placeholder="New name"
        initialValue={renameTarget?.name ?? ""}
        confirmLabel="Rename"
        busy={busy}
        onClose={() => setRenameTarget(null)}
        onConfirm={(name) => void doRename(name)}
      />
      <Toaster />
      <UploadPanel jobs={uploads} onClose={clearUploads} onDismiss={dismissUpload} />
    </>
  );
}
