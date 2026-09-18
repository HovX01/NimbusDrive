/** Pull a URL out of Web Share Target query params (X often puts the link in `text`). */
export function extractSharedURL(search: string): string | null {
  const q = new URLSearchParams(search.startsWith("?") ? search.slice(1) : search);
  for (const key of ["url", "link"]) {
    const v = (q.get(key) || "").trim();
    if (isHttpURL(v)) return cleanURL(v);
  }
  const blob = [q.get("text"), q.get("title"), q.get("url")].filter(Boolean).join("\n");
  const m = blob.match(/https?:\/\/[^\s<>"'`]+/i);
  if (!m) return null;
  return cleanURL(m[0]);
}

function isHttpURL(s: string) {
  try {
    const u = new URL(s);
    return u.protocol === "http:" || u.protocol === "https:";
  } catch {
    return false;
  }
}

function cleanURL(s: string) {
  return s.replace(/[),.;]+$/g, "");
}

/** True when the current path is the share-target landing page. */
export function isShareTargetPath(pathname: string) {
  return pathname === "/share-target" || pathname.endsWith("/share-target");
}
