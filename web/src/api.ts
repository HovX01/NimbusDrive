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

export async function listFiles(token: string, parentId: string) {
  const q = parentId ? `?parent_id=${encodeURIComponent(parentId)}` : "";
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
