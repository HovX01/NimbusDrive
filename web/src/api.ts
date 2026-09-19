const API = import.meta.env.VITE_API_BASE ?? "";

export const ACCESS_KEY = "nimbus_access";

export type User = {
  telegram_id: number;
  username: string;
  first_name: string;
  phone: string;
  has_avatar?: boolean;
};

export type Node = {
  id: string;
  parent_id?: string;
  name: string;
  type: "file" | "folder";
  mime_type: string;
  size: number;
  status: string;
  duplicate?: boolean;
  url?: string;
};

export type PathCrumb = { id: string; name: string };

export type SearchHit = Node & {
  path: PathCrumb[];
  rank: number;
};

export function getAccessKey(): string | null {
  return localStorage.getItem(ACCESS_KEY);
}

export function setAccessKey(key: string) {
  localStorage.setItem(ACCESS_KEY, key.trim());
}

export function clearAccessKey() {
  localStorage.removeItem(ACCESS_KEY);
}

function accessHeaders(): HeadersInit {
  const key = getAccessKey();
  return key ? { "X-Nimbus-Access": key } : {};
}

function authHeaders(token: string | null): HeadersInit {
  return {
    ...accessHeaders(),
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  };
}

async function parse<T>(res: Response): Promise<T> {
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const msg = data?.error?.message ?? res.statusText;
    throw new Error(msg);
  }
  return data as T;
}

export async function setupStatus() {
  return parse<{ configured: boolean; authorized: boolean; access_key_required?: boolean }>(
    await fetch(`${API}/api/v1/setup/status`),
  );
}

export async function validateAccessKey(key: string) {
  const res = await fetch(`${API}/api/v1/auth/resume`, {
    method: "POST",
    headers: key ? { "X-Nimbus-Access": key.trim() } : {},
  });
  // Only a 401 carrying the access-key message means the key was rejected.
  // Other failures (no telegram session yet, not configured) mean the key
  // passed the gate — the access middleware and ErrNotAuthenticated both 401.
  if (res.status === 401) {
    const body = await res.json().catch(() => ({}));
    const msg = body?.error?.message ?? "";
    if (msg.toLowerCase().includes("access key")) {
      throw new Error(msg);
    }
    return;
  }
  await res.json().catch(() => ({}));
}

export async function setupTelegram(apiId: number, apiHash: string) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/setup/telegram`, {
      method: "POST",
      headers: { ...accessHeaders(), "Content-Type": "application/json" },
      body: JSON.stringify({ api_id: apiId, api_hash: apiHash }),
    }),
  );
}

export async function resumeSession() {
  return parse<{ token: string; user: User }>(
    await fetch(`${API}/api/v1/auth/resume`, {
      method: "POST",
      headers: accessHeaders(),
    }),
  );
}

export async function sendCode(phone: string) {
  return parse<{ phone_code_hash: string }>(
    await fetch(`${API}/api/v1/auth/send-code`, {
      method: "POST",
      headers: { ...accessHeaders(), "Content-Type": "application/json" },
      body: JSON.stringify({ phone }),
    }),
  );
}

export async function signIn(body: {
  phone: string;
  code: string;
  phone_code_hash: string;
  password?: string;
}) {
  return parse<{ token: string; user: User }>(
    await fetch(`${API}/api/v1/auth/sign-in`, {
      method: "POST",
      headers: { ...accessHeaders(), "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export async function me(token: string) {
  return parse<User>(
    await fetch(`${API}/api/v1/auth/me`, { headers: authHeaders(token) }),
  );
}

export async function logout(token: string) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/auth/logout`, {
      method: "POST",
      headers: authHeaders(token),
    }),
  );
}

export async function listFiles(token: string, parentId: string, options?: { recursive?: boolean; limit?: number }) {
  const params = new URLSearchParams();
  if (parentId) params.set("parent_id", parentId);
  if (options?.recursive) params.set("recursive", "true");
  if (options?.limit) params.set("limit", String(options.limit));
  const q = params.toString() ? `?${params.toString()}` : "";
  return parse<{ items: Node[] }>(
    await fetch(`${API}/api/v1/files${q}`, { headers: authHeaders(token) }),
  );
}

export async function searchFiles(token: string, query: string) {
  const q = `?q=${encodeURIComponent(query)}`;
  return parse<{ items: SearchHit[] }>(
    await fetch(`${API}/api/v1/search${q}`, { headers: authHeaders(token) }),
  );
}

export async function mkdir(token: string, parentId: string, name: string) {
  return parse<Node>(
    await fetch(`${API}/api/v1/folders`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ parent_id: parentId, name }),
    }),
  );
}

export async function uploadFile(
  token: string,
  parentId: string,
  file: File,
  onProgress?: (pct: number) => void,
  onBodySent?: () => void,
) {
  const fd = new FormData();
  fd.append("file", file);
  if (parentId) fd.append("parent_id", parentId);

  return new Promise<Node>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${API}/api/v1/files/upload`);
    const access = getAccessKey();
    if (access) xhr.setRequestHeader("X-Nimbus-Access", access);
    xhr.setRequestHeader("Authorization", `Bearer ${token}`);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) {
        onProgress(Math.min(99, Math.round((e.loaded / e.total) * 100)));
      }
    };
    xhr.upload.onload = () => {
      onProgress?.(100);
      onBodySent?.();
    };
    xhr.onload = () => {
      let data: { error?: { message?: string } } & Partial<Node> = {};
      try {
        data = JSON.parse(xhr.responseText);
      } catch {
        /* ignore */
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(data as Node);
        return;
      }
      reject(new Error(data?.error?.message ?? (xhr.statusText || "Upload failed")));
    };
    xhr.onerror = () => reject(new Error("Upload failed"));
    xhr.send(fd);
  });
}

export async function renameNode(token: string, id: string, name: string) {
  return parse<Node>(
    await fetch(`${API}/api/v1/files/${id}`, {
      method: "PATCH",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    }),
  );
}

export async function moveNode(token: string, id: string, parentId: string) {
  return parse<Node>(
    await fetch(`${API}/api/v1/files/${id}/move`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ parent_id: parentId }),
    }),
  );
}

export async function moveNodes(token: string, ids: string[], parentId: string) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/files/move`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ ids, parent_id: parentId }),
    }),
  );
}

export async function deleteNode(token: string, id: string) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/files/${id}`, {
      method: "DELETE",
      headers: authHeaders(token),
    }),
  );
}

export async function listTrash(token: string) {
  return parse<{ items: Node[] }>(
    await fetch(`${API}/api/v1/trash`, { headers: authHeaders(token) }),
  );
}

export async function purgeNode(token: string, id: string) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/trash/${id}`, {
      method: "DELETE",
      headers: authHeaders(token),
    }),
  );
}

export async function emptyTrash(token: string) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/trash`, {
      method: "DELETE",
      headers: authHeaders(token),
    }),
  );
}

export type TelegramContact = {
  id: number;
  username?: string;
  first_name: string;
  last_name?: string;
  display_name: string;
  has_avatar: boolean;
  /** Present when this recipient is an allowed Telegram bot. */
  is_bot?: boolean;
};

export async function listContacts(token: string, query = "") {
  const q = query.trim() ? `?q=${encodeURIComponent(query.trim())}` : "";
  return parse<{ items: TelegramContact[] }>(
    await fetch(`${API}/api/v1/contacts${q}`, { headers: authHeaders(token) }),
  );
}

export async function sendToTelegram(token: string, fileId: string, userId: number) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/files/${fileId}/send-telegram`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ user_id: userId }),
    }),
  );
}

export function contactAvatarUrl(id: number) {
  return `${API}/api/v1/contacts/${id}/avatar`;
}

export async function fetchContactAvatarBlob(token: string, id: number): Promise<Blob> {
  const res = await fetch(contactAvatarUrl(id), { headers: authHeaders(token) });
  if (!res.ok) throw new Error("No avatar");
  return res.blob();
}

export type ShareInfo = {
  link: {
    id: string;
    token: string;
    node_id: string;
    created_at: string;
    download_count: number;
  };
  url: string;
};

export async function createShare(token: string, fileId: string) {
  return parse<ShareInfo>(
    await fetch(`${API}/api/v1/files/${fileId}/share`, {
      method: "POST",
      headers: authHeaders(token),
    }),
  );
}

export async function listShares(token: string, fileId: string) {
  return parse<{ items: ShareInfo[] }>(
    await fetch(`${API}/api/v1/files/${fileId}/shares`, { headers: authHeaders(token) }),
  );
}

export async function revokeShare(token: string, shareId: string) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/shares/${shareId}`, {
      method: "DELETE",
      headers: authHeaders(token),
    }),
  );
}

export function publicShareUrl(token: string) {
  return `${API}/api/v1/share/${token}/download`;
}

export type StoredFile = Node & { url?: string };

export async function fetchStorageSettings(token: string) {
  return parse<{ api_key: string; api_base: string }>(
    await fetch(`${API}/api/v1/settings/storage`, { headers: authHeaders(token) }),
  );
}

export type StorageKindStats = { kind: string; count: number; bytes: number };
export type StorageBucketStats = { channel_id: number; count: number; bytes: number };
export type DayPoint = { day: string; count: number; bytes: number };

export type StorageStats = {
  total_bytes: number;
  total_files: number;
  total_folders: number;
  trash_bytes: number;
  trash_files: number;
  by_kind: StorageKindStats[] | null;
  by_channel: StorageBucketStats[] | null;
  daily: DayPoint[] | null;
};

export async function fetchStorageStats(token: string, days = 30) {
  return parse<StorageStats>(
    await fetch(`${API}/api/v1/stats/storage?days=${days}`, { headers: authHeaders(token) }),
  );
}

export type BackupSettings = {
  enabled: boolean;
  endpoint: string;
  region: string;
  bucket: string;
  prefix: string;
  use_ssl: boolean;
  path_style: boolean;
  credentials_configured: boolean;
  access_key_id_hint?: string;
};

export async function fetchBackupSettings(token: string) {
  return parse<BackupSettings>(
    await fetch(`${API}/api/v1/settings/backup`, { headers: authHeaders(token) }),
  );
}

export async function saveBackupSettings(
  token: string,
  body: {
    enabled: boolean;
    endpoint: string;
    region: string;
    bucket: string;
    prefix: string;
    access_key_id: string;
    secret_key: string;
    use_ssl: boolean;
    path_style: boolean;
  },
) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/settings/backup`, {
      method: "PUT",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export async function testBackupSettings(token: string) {
  return parse<{ ok: boolean; message: string }>(
    await fetch(`${API}/api/v1/settings/backup/test`, {
      method: "POST",
      headers: authHeaders(token),
    }),
  );
}

export async function startBackup(token: string, type: string) {
  return parse<{ job_id: string; status: string }>(
    await fetch(`${API}/api/v1/backups`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ type }),
    }),
  );
}

export async function backupStatus(token: string, jobId: string) {
  return parse<{ status: string; progress: number; message: string }>(
    await fetch(`${API}/api/v1/backups/${encodeURIComponent(jobId)}`, {
      headers: authHeaders(token),
    }),
  );
}

export type S3Settings = {
  enabled: boolean;
  region: string;
  access_key: string;
  secret_key: string;
  endpoint: string;
  rclone: string;
};

export async function fetchS3Settings(token: string) {
  return parse<S3Settings>(
    await fetch(`${API}/api/v1/settings/s3`, { headers: authHeaders(token) }),
  );
}

export async function saveS3Settings(
  token: string,
  body: { region?: string; regenerate?: boolean },
) {
  return parse<S3Settings>(
    await fetch(`${API}/api/v1/settings/s3`, {
      method: "PUT",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export type S3Bucket = { name: string; created_at: string };

export type S3Object = { key: string; size: number; mime_type: string; modified: string };

export async function fetchS3Buckets(token: string) {
  return parse<{ items: S3Bucket[] }>(
    await fetch(`${API}/api/v1/s3/buckets`, { headers: authHeaders(token) }),
  );
}

export async function createS3Bucket(token: string, name: string) {
  return parse<S3Bucket>(
    await fetch(`${API}/api/v1/s3/buckets`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    }),
  );
}

export async function fetchS3BucketObjects(token: string, bucket: string) {
  return parse<{ bucket: string; items: S3Object[]; upload: string }>(
    await fetch(`${API}/api/v1/s3/buckets/${encodeURIComponent(bucket)}`, {
      headers: authHeaders(token),
    }),
  );
}

export type TelegramBot = {
  id: number;
  username?: string;
  display_name: string;
  has_avatar: boolean;
  allowed: boolean;
};

export async function listBots(token: string) {
  return parse<{ items: TelegramBot[] }>(
    await fetch(`${API}/api/v1/bots`, { headers: authHeaders(token) }),
  );
}

export async function setBotAllowed(token: string, botId: number, allowed: boolean) {
  return parse<TelegramBot>(
    await fetch(`${API}/api/v1/bots/${botId}`, {
      method: "PATCH",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ allowed }),
    }),
  );
}

export type SocialProvider = "tiktok" | "instagram" | "facebook";

export type SocialConnection = {
  id?: string;
  provider: SocialProvider;
  account_id?: string;
  auth_type?: string;
  display_name?: string;
  username?: string;
  scopes?: string[];
  connected: boolean;
  enabled: boolean;
  active: boolean;
  expires_at?: string;
  updated_at: string;
};

export type SocialOAuthAppConfig = {
  provider: SocialProvider;
  client_id?: string;
  client_secret?: string;
  public_base_url?: string;
  configured: boolean;
  updated_at: string;
};

export async function listSocialConnections(token: string) {
  return parse<{ items: SocialConnection[] }>(
    await fetch(`${API}/api/v1/social/connections`, { headers: authHeaders(token) }),
  );
}

export async function listSocialOAuthConfigs(token: string) {
  return parse<{ items: SocialOAuthAppConfig[] }>(
    await fetch(`${API}/api/v1/social/oauth/config`, { headers: authHeaders(token) }),
  );
}

export async function saveSocialOAuthConfig(
  token: string,
  provider: SocialProvider,
  body: { client_id: string; client_secret: string; public_base_url: string },
) {
  return parse<SocialOAuthAppConfig>(
    await fetch(`${API}/api/v1/social/oauth/config/${provider}`, {
      method: "PUT",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export async function addSocialCookieConnection(
  token: string,
  provider: SocialProvider,
  displayName: string,
  file: Blob,
) {
  const fd = new FormData();
  fd.append("cookies", file, "cookies.txt");
  if (displayName.trim()) fd.append("display_name", displayName.trim());
  return parse<SocialConnection>(
    await fetch(`${API}/api/v1/social/connections/${provider}/cookies`, {
      method: "POST",
      headers: authHeaders(token),
      body: fd,
    }),
  );
}

export async function startSocialOAuth(token: string, provider: SocialProvider) {
  return parse<{ url: string }>(
    await fetch(`${API}/api/v1/social/oauth/${provider}/start`, { headers: authHeaders(token) }),
  );
}

export function socialOAuthStartUrl(token: string, provider: SocialProvider) {
  const q = new URLSearchParams({ token, redirect: "1" });
  const access = getAccessKey();
  if (access) q.set("access", access);
  return `${API}/api/v1/social/oauth/${provider}/start?${q}`;
}

export async function setSocialConnectionEnabled(token: string, provider: SocialProvider, enabled: boolean) {
  return parse<SocialConnection>(
    await fetch(`${API}/api/v1/social/connections/${provider}`, {
      method: "PATCH",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ enabled }),
    }),
  );
}

export async function setActiveSocialConnection(token: string, provider: SocialProvider, id: string) {
  return parse<SocialConnection>(
    await fetch(`${API}/api/v1/social/connections/${provider}/${encodeURIComponent(id)}/active`, {
      method: "POST",
      headers: authHeaders(token),
    }),
  );
}

export async function disconnectSocialConnection(token: string, provider: SocialProvider, id: string) {
  return parse<{ status: string }>(
    await fetch(`${API}/api/v1/social/connections/${provider}/${encodeURIComponent(id)}`, {
      method: "DELETE",
      headers: authHeaders(token),
    }),
  );
}

export type MediaAction = "enhance" | "compress" | "playable" | "extract_audio";

export type MediaJobStatus = {
  id: string;
  action: MediaAction | string;
  status: "queued" | "running" | "done" | "error" | string;
  phase: string;
  progress: number;
  message: string;
  node?: Node;
};

export type EditProject = {
  id: string;
  source_node_id: string;
  name: string;
  timeline_json: string;
  created_at: string;
  updated_at: string;
};

export type TimelineData = {
  duration: number;
  tracks: TimelineTrack[];
};

export type TimelineTrack = {
  id: string;
  type: "video" | "audio" | "text" | "caption" | "image";
  clips: TimelineClip[];
  muted?: boolean;
};

export type TimelineClip = {
  id: string;
  sourceNodeId: string;
  startInSource: number;
  endInSource: number;
  startOnTimeline: number;
  speed?: number;
  hasEffects?: boolean;
  volume?: number;
  fade_in?: number;
  fade_out?: number;
  text?: TextOverlay;
  transition?: Transition;
};

export type ExportPreset = "match" | "1080p" | "compressed";

export type TextOverlay = {
  content: string;
  font_size: number;
  font_family: string;
  color: string;
  bg_color: string;
  x: number;
  y: number;
  alignment: "center" | "left" | "right";
  animation: "none" | "fade" | "slide-up" | "typewriter";
};

export type Transition = {
  type: "crossfade" | "fade-black" | "wipe-left";
  duration: number;
};

export type FileProbe = {
  duration: number;
  width: number;
  height: number;
  video_codec: string;
  audio_codec: string;
  has_audio: boolean;
};

export type Caption = {
  index: number;
  start_time: number;
  end_time: number;
  text: string;
};

export type EditExportJobStatus = {
  id: string;
  project_id: string;
  status: "queued" | "running" | "done" | "error" | string;
  phase: string;
  progress: number;
  message: string;
  node?: Node;
};

export async function startMediaJob(token: string, fileId: string, action: MediaAction) {
  return parse<{ job_id: string }>(
    await fetch(`${API}/api/v1/files/${fileId}/media`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ action }),
    }),
  );
}

export async function mediaJobStatus(token: string, jobId: string) {
  return parse<MediaJobStatus>(
    await fetch(`${API}/api/v1/files/media/${encodeURIComponent(jobId)}`, {
      headers: authHeaders(token),
    }),
  );
}

export async function createEditProject(token: string, fileId: string) {
  return parse<{ project: EditProject }>(
    await fetch(`${API}/api/v1/files/${fileId}/edit/projects`, {
      method: "POST",
      headers: authHeaders(token),
    }),
  );
}

export async function listEditProjects(token: string) {
  return parse<{ projects: EditProject[] }>(
    await fetch(`${API}/api/v1/edit/projects`, { headers: authHeaders(token) }),
  );
}

export async function getEditProject(token: string, projectId: string) {
  return parse<{ project: EditProject }>(
    await fetch(`${API}/api/v1/edit/projects/${encodeURIComponent(projectId)}`, {
      headers: authHeaders(token),
    }),
  );
}

export async function saveEditTimeline(token: string, projectId: string, timeline: TimelineData) {
  return parse<{ ok: boolean }>(
    await fetch(`${API}/api/v1/edit/projects/${encodeURIComponent(projectId)}`, {
      method: "PATCH",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ timeline_json: JSON.stringify(timeline) }),
    }),
  );
}

export function saveEditTimelineBeacon(token: string, projectId: string, timeline: TimelineData) {
  const params = new URLSearchParams({ token });
  const access = getAccessKey();
  if (access) params.set("access", access);
  const body = JSON.stringify({ timeline_json: JSON.stringify(timeline) });
  const blob = new Blob([body], { type: "application/json" });
  return navigator.sendBeacon?.(`${API}/api/v1/edit/projects/${encodeURIComponent(projectId)}?${params}`, blob) ?? false;
}

export async function deleteEditProject(token: string, projectId: string) {
  const res = await fetch(`${API}/api/v1/edit/projects/${encodeURIComponent(projectId)}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) await parse(res);
}

export async function startEditExport(token: string, projectId: string, preset: ExportPreset) {
  return parse<{ job_id: string }>(
    await fetch(`${API}/api/v1/edit/projects/${encodeURIComponent(projectId)}/export`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ preset }),
    }),
  );
}

export async function getEditExportStatus(token: string, jobId: string) {
  return parse<EditExportJobStatus>(
    await fetch(`${API}/api/v1/edit/jobs/${encodeURIComponent(jobId)}`, {
      headers: authHeaders(token),
    }),
  );
}

export async function probeFile(token: string, fileId: string) {
  return parse<FileProbe>(
    await fetch(`${API}/api/v1/files/${encodeURIComponent(fileId)}/probe`, {
      headers: authHeaders(token),
    }),
  );
}

export async function getKeyframes(token: string, fileId: string, start = 0, end = 0) {
  const q = new URLSearchParams({ start: String(start), end: String(end) });
  return parse<{ keyframes: number[] }>(
    await fetch(`${API}/api/v1/files/${encodeURIComponent(fileId)}/keyframes?${q}`, {
      headers: authHeaders(token),
    }),
  );
}

export async function getProxyStatus(token: string, id: string) {
  return parse<{ status: "none" | "generating" | "ready" | "error"; message?: string }>(
    await fetch(`${API}/api/v1/files/${encodeURIComponent(id)}/proxy/status`, {
      headers: authHeaders(token),
    }),
  );
}

export function proxyStreamUrl(token: string, id: string) {
  const q = new URLSearchParams({ token });
  const access = getAccessKey();
  if (access) q.set("access", access);
  return `${API}/api/v1/files/${encodeURIComponent(id)}/proxy?${q}`;
}

export async function importCaptions(token: string, projectId: string, file: File) {
  const fd = new FormData();
  fd.append("file", file);
  return parse<{ captions: Caption[] }>(
    await fetch(`${API}/api/v1/edit/projects/${encodeURIComponent(projectId)}/captions/import`, {
      method: "POST",
      headers: authHeaders(token),
      body: fd,
    }),
  );
}

export function captionExportUrl(token: string, projectId: string) {
  const q = new URLSearchParams({ token });
  const access = getAccessKey();
  if (access) q.set("access", access);
  return `${API}/api/v1/edit/projects/${encodeURIComponent(projectId)}/captions/export?${q}`;
}

export async function importFileURL(token: string, parentId: string, url: string) {
  return parse<StoredFile>(
    await fetch(`${API}/api/v1/files/import`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ parent_id: parentId, url, public: true }),
    }),
  );
}

export async function fetchFromURL(
  token: string,
  parentId: string,
  body: { url: string; mode: "video" | "audio"; max_height?: number | null },
) {
  return parse<{ job_id: string }>(
    await fetch(`${API}/api/v1/files/fetch`, {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({
        parent_id: parentId,
        url: body.url,
        mode: body.mode,
        max_height: body.max_height ?? 0,
      }),
    }),
  );
}

export type FetchJobStatus = {
  id: string;
  status: "queued" | "running" | "done" | "error" | string;
  phase: string;
  progress: number;
  message: string;
  error_code?: string;
  service?: string;
  node?: Node;
  nodes?: Node[];
  count?: number;
};

export async function fetchJobStatus(token: string, jobId: string) {
  return parse<FetchJobStatus>(
    await fetch(`${API}/api/v1/files/fetch/${encodeURIComponent(jobId)}`, {
      headers: authHeaders(token),
    }),
  );
}

export function downloadUrl(id: string) {
  return `${API}/api/v1/files/${id}/download`;
}

/** Authenticated media URL for <video>/<audio> (supports Range streaming). */
export function mediaStreamUrl(token: string, id: string) {
  const base = downloadUrl(id);
  const q = new URLSearchParams({ token });
  const access = getAccessKey();
  if (access) q.set("access", access);
  return `${base}?${q}`;
}

export function thumbUrl(id: string) {
  return `${API}/api/v1/files/${id}/thumb`;
}

export function avatarUrl() {
  return `${API}/api/v1/auth/avatar`;
}

/** Fetch file bytes with auth for in-browser preview. */
export async function fetchFileBlob(token: string, id: string): Promise<Blob> {
  const res = await fetch(downloadUrl(id), { headers: authHeaders(token) });
  if (!res.ok) throw new Error("Could not load preview");
  return res.blob();
}

/** Small cached JPEG for grid cards (fast). */
export async function fetchThumbBlob(token: string, id: string): Promise<Blob> {
  const res = await fetch(thumbUrl(id), { headers: authHeaders(token) });
  if (!res.ok) throw new Error("No thumb");
  return res.blob();
}

export async function fetchAvatarBlob(token: string): Promise<Blob> {
  const res = await fetch(avatarUrl(), { headers: authHeaders(token) });
  if (!res.ok) throw new Error("No avatar");
  return res.blob();
}
