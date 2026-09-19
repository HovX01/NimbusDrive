import { useEffect, useRef, useState } from "react";
import {
  Archive,
  File as FileIcon,
  FileImage,
  FileText,
  FileVideo,
  Folder,
  Music,
  Play,
} from "lucide-react";
import { fetchThumbBlob } from "../api";
import { previewKind, type PreviewKind } from "../lib/files";
import { cn } from "@/lib/utils";

const ICON: Record<PreviewKind, typeof FileIcon> = {
  folder: Folder,
  image: FileImage,
  video: FileVideo,
  audio: Music,
  pdf: FileText,
  text: FileText,
  archive: Archive,
  file: FileIcon,
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
  const Icon = ICON[kind];

  return (
    <div
      ref={ref}
      aria-hidden
      className={cn(
        "relative grid w-full place-items-center overflow-hidden bg-muted/60 text-muted-foreground",
        compact ? "h-10 w-10 shrink-0 rounded-xl" : "aspect-[16/10] rounded-none",
        showMedia && "bg-zinc-900 text-white",
        isFolder && "bg-amber-50 text-amber-600",
      )}
    >
      {showMedia ? (
        <>
          <img src={src!} alt="" className="h-full w-full object-cover" loading="lazy" />
          {kind === "video" && (
            <span className="absolute bottom-1.5 left-1.5 grid h-7 w-7 place-items-center rounded-full bg-black/70 text-white">
              <Play className="h-3 w-3 fill-current" />
            </span>
          )}
        </>
      ) : (
        <Icon className={cn(compact ? "h-4 w-4" : "h-7 w-7")} strokeWidth={1.75} />
      )}
    </div>
  );
}

export function KindIcon({ kind, className }: { kind: PreviewKind; className?: string }) {
  const Icon = ICON[kind];
  return <Icon aria-hidden className={cn("h-3.5 w-3.5 text-muted-foreground", className)} />;
}
