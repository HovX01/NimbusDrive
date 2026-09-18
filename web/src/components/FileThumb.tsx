import { useEffect, useRef, useState } from "react";
import { fetchThumbBlob } from "../api";
import { previewKind, type PreviewKind } from "../lib/files";

const FA: Record<PreviewKind, string> = {
  folder: "fa-solid fa-folder",
  image: "fa-solid fa-image",
  video: "fa-solid fa-film",
  audio: "fa-solid fa-music",
  pdf: "fa-solid fa-file-pdf",
  text: "fa-solid fa-file-lines",
  archive: "fa-solid fa-file-zipper",
  file: "fa-solid fa-file",
};

export function FileThumb({
  token,
  id,
  name,
  mime,
  isFolder,
  compact = false,
}: {
  token: string;
  id: string;
  name: string;
  mime: string;
  isFolder: boolean;
  compact?: boolean;
}) {
  const kind = previewKind(name, mime, isFolder);
  const [src, setSrc] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isFolder || (kind !== "image" && kind !== "video")) return;

    let cancelled = false;
    let objectUrl: string | null = null;
    let started = false;

    const load = () => {
      if (started || cancelled) return;
      started = true;
      fetchThumbBlob(token, id)
        .then((blob) => {
          if (cancelled) return;
          objectUrl = URL.createObjectURL(blob);
          setSrc(objectUrl);
          setFailed(false);
        })
        .catch(() => {
          if (!cancelled) setFailed(true);
        });
    };

    const el = ref.current;
    if (!el || typeof IntersectionObserver === "undefined") {
      load();
      return () => {
        cancelled = true;
        if (objectUrl) URL.revokeObjectURL(objectUrl);
      };
    }

    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          io.disconnect();
          load();
        }
      },
      { rootMargin: "200px" },
    );
    io.observe(el);

    const rect = el.getBoundingClientRect();
    if (rect.bottom > -200 && rect.top < window.innerHeight + 200) {
      io.disconnect();
      load();
    }

    return () => {
      cancelled = true;
      io.disconnect();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [token, id, isFolder, kind]);

  const showMedia = (kind === "image" || kind === "video") && !!src && !failed;

  return (
    <div
      ref={ref}
      className={`file-badge kind-${kind} ${compact ? "compact" : ""} ${showMedia ? "has-media" : ""}`}
      aria-hidden
    >
      {showMedia && <img src={src!} alt="" className="thumb-media" />}
      {showMedia && kind === "video" && (
        <span className="thumb-play" aria-hidden>
          <i className="fa-solid fa-play" />
        </span>
      )}
      {!showMedia && <i className={`${FA[kind]} fa-icon`} />}
    </div>
  );
}

export function KindIcon({ kind }: { kind: PreviewKind }) {
  return <i className={`${FA[kind]} fa-inline`} aria-hidden />;
}
