export type PreviewKind =
  | "image"
  | "video"
  | "audio"
  | "pdf"
  | "text"
  | "archive"
  | "folder"
  | "file";

const IMAGE_EXT = new Set([
  "png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "avif", "heic", "ico",
]);
const VIDEO_EXT = new Set([
  "mp4", "webm", "mov", "mkv", "m4v", "avi", "ogv",
]);
const AUDIO_EXT = new Set([
  "mp3", "wav", "ogg", "m4a", "flac", "aac", "opus",
]);
const PDF_EXT = new Set(["pdf"]);
const TEXT_EXT = new Set([
  "txt", "md", "json", "csv", "log", "xml", "yml", "yaml", "ts", "tsx", "js", "jsx", "css", "html", "go", "rs", "py",
]);
const ARCHIVE_EXT = new Set(["zip", "rar", "7z", "tar", "gz"]);

export function extOf(name: string): string {
  const i = name.lastIndexOf(".");
  if (i < 0) return "";
  return name.slice(i + 1).toLowerCase();
}

export function previewKind(name: string, mime: string, isFolder: boolean): PreviewKind {
  if (isFolder) return "folder";
  if (mime.startsWith("image/")) return "image";
  if (mime.startsWith("video/")) return "video";
  if (mime.startsWith("audio/")) return "audio";
  if (mime === "application/pdf") return "pdf";
  const ext = extOf(name);
  if (IMAGE_EXT.has(ext)) return "image";
  if (VIDEO_EXT.has(ext)) return "video";
  if (AUDIO_EXT.has(ext)) return "audio";
  if (PDF_EXT.has(ext)) return "pdf";
  if (TEXT_EXT.has(ext)) return "text";
  if (ARCHIVE_EXT.has(ext)) return "archive";
  return "file";
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  return `${(n / 1024 ** 3).toFixed(2)} GB`;
}

export function kindLabel(kind: PreviewKind): string {
  switch (kind) {
    case "folder":
      return "Folder";
    case "image":
      return "Image";
    case "video":
      return "Video";
    case "audio":
      return "Audio";
    case "pdf":
      return "PDF";
    case "text":
      return "Document";
    case "archive":
      return "Archive";
    default:
      return "File";
  }
}

/** Files that open in the preview modal (not folders/archives/generic). */
export function isPreviewable(name: string, mime: string, isFolder: boolean): boolean {
  if (isFolder) return false;
  const k = previewKind(name, mime, false);
  return k !== "file" && k !== "archive";
}

/** Full-screen dark viewer (Nextcloud-style slideshow shell). */
export function usesFullscreenViewer(kind: PreviewKind): boolean {
  return kind === "image" || kind === "video" || kind === "pdf";
}
