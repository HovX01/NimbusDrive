import { useEffect, useState } from "react";
import {
  ChevronLeft,
  ChevronRight,
  Download,
  Film,
  Link2,
  Loader2,
  Music,
  PenLine,
  Scissors,
  Send,
  Trash2,
} from "lucide-react";
import { fetchFileBlob, fetchThumbBlob, mediaStreamUrl, type Node } from "../api";
import { extOf, formatBytes, kindLabel, previewKind } from "../lib/files";
import { cn } from "@/lib/utils";
import { FileThumb } from "./FileThumb";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import { Skeleton } from "./ui/skeleton";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./ui/tooltip";

type Props = {
  token: string;
  node: Node;
  playlist: Node[];
  index: number;
  onNavigate: (index: number) => void;
  onClose: () => void;
  onDownload: () => void;
  onRename: () => void;
  onShare: () => void;
  onSendTelegram: () => void;
  onMediaStudio?: () => void;
  onEditVideo?: () => void;
  onDelete: () => void;
};

function likelyHEVC(name: string, mime: string) {
  const n = name.toLowerCase();
  const m = mime.toLowerCase();
  if (n.includes("screenrecording") || n.includes("screen-recording") || n.includes("screen_recording")) {
    return true;
  }
  if (extOf(name) === "mov" && (m.includes("quicktime") || m === "" || m === "application/octet-stream")) {
    return true;
  }
  return m.includes("hevc") || m.includes("h265");
}

function isHEIC(name: string, mime: string) {
  const m = mime.toLowerCase();
  const e = extOf(name);
  return e === "heic" || e === "heif" || m.includes("heic") || m.includes("heif");
}

function PreviewContent({
  token,
  node,
  kind,
  onDownload,
  onMediaStudio,
}: {
  token: string;
  node: Node;
  kind: ReturnType<typeof previewKind>;
  onDownload: () => void;
  onMediaStudio?: () => void;
}) {
  const [url, setUrl] = useState<string | null>(null);
  const [thumbUrl, setThumbUrl] = useState<string | null>(null);
  const [text, setText] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [buffering, setBuffering] = useState(false);
  const [playError, setPlayError] = useState("");
  const hevc = kind === "video" && likelyHEVC(node.name, node.mime_type);
  const heic = kind === "image" && isHEIC(node.name, node.mime_type);

  useEffect(() => {
    const urls: string[] = [];
    let cancelled = false;
    setLoading(true);
    setError("");
    setPlayError("");
    setBuffering(false);
    setText(null);
    setUrl(null);
    setThumbUrl(null);

    if (kind === "video" || kind === "audio") {
      if (kind === "video" && node.size < 1024) {
        setError("This video file is incomplete (stored size under 1 KB).");
        setLoading(false);
        return;
      }
      setUrl(mediaStreamUrl(token, node.id));
      setLoading(false);
      setBuffering(true);
      return;
    }

    if (kind === "image") {
      if (heic) {
        setError("HEIC photos need conversion — use Media Studio or download to view.");
        setLoading(false);
        return;
      }
      setUrl(mediaStreamUrl(token, node.id));
      setLoading(false);
      void fetchThumbBlob(token, node.id)
        .then((thumb) => {
          if (cancelled) return;
          const u = URL.createObjectURL(thumb);
          urls.push(u);
          setThumbUrl(u);
        })
        .catch(() => undefined);
      return;
    }

    (async () => {
      try {
        const blob = await fetchFileBlob(token, node.id);
        if (cancelled) return;
        if (kind === "text") {
          setText(await blob.text());
        } else if (kind === "pdf") {
          const objectUrl = URL.createObjectURL(blob);
          urls.push(objectUrl);
          setUrl(objectUrl);
        }
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
      for (const u of urls) URL.revokeObjectURL(u);
    };
  }, [token, node.id, node.size, kind, heic]);

  if (loading) {
    return (
      <div className="grid place-items-center gap-2 py-16" aria-busy="true">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        <p className="text-sm text-muted-foreground">Loading preview…</p>
        <Skeleton className="h-[280px] w-full max-w-[640px]" />
      </div>
    );
  }
  if (error) {
    return (
      <div className="grid justify-items-center gap-2 rounded-2xl bg-muted/50 px-6 py-12 text-center">
        <p className="font-semibold">Preview unavailable</p>
        <p className="max-w-md text-sm text-muted-foreground">{error}</p>
        <Button onClick={onDownload} className="mt-2">
          <Download /> Download
        </Button>
      </div>
    );
  }

  if (kind === "image" && url) {
    return (
      <div className="relative grid place-items-center overflow-hidden rounded-2xl bg-zinc-950">
        {thumbUrl && (
          <img src={thumbUrl} alt="" aria-hidden className="absolute inset-0 h-full w-full scale-105 object-cover opacity-40 blur-xl" />
        )}
        <img
          src={url}
          alt={node.name}
          className="relative max-h-[62vh] w-auto max-w-full object-contain"
          onLoad={() => setThumbUrl(null)}
        />
      </div>
    );
  }

  if (kind === "video" && url) {
    if (playError) {
      return (
        <div className="grid justify-items-center gap-2 rounded-2xl bg-muted/50 px-6 py-12 text-center">
          <p className="font-semibold">Can’t play this video here</p>
          <p className="max-w-md text-sm text-muted-foreground">
            {playError}
            {hevc ? " iPhone screen recordings are usually HEVC — try Make playable." : ""}
          </p>
          <div className="mt-2 flex flex-wrap justify-center gap-2">
            {onMediaStudio && (
              <Button onClick={onMediaStudio}>
                <Film /> Make playable
              </Button>
            )}
            <Button variant="outline" onClick={onDownload}>
              <Download /> Download
            </Button>
          </div>
        </div>
      );
    }
    return (
      <div className="grid gap-2 overflow-hidden rounded-2xl bg-zinc-950 p-2">
        {buffering && (
          <p className="flex items-center justify-center gap-2 px-2 py-1 text-xs text-zinc-400">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            {hevc ? "Loading… if blank, use Make playable (HEVC)." : "Loading from Telegram…"}
          </p>
        )}
        <video
          key={node.id}
          src={url}
          controls
          autoPlay
          playsInline
          preload="auto"
          className="max-h-[62vh] w-full rounded-xl bg-black"
          onLoadStart={() => setBuffering(true)}
          onWaiting={() => setBuffering(true)}
          onPlaying={() => setBuffering(false)}
          onCanPlay={() => setBuffering(false)}
          onError={(e) => {
            const media = e.currentTarget;
            const code = media.error?.code;
            if (code === 2) {
              setPlayError("Network hiccup while streaming. Close and open again, or try Make playable.");
              return;
            }
            setPlayError(hevc ? "This looks like an HEVC screen recording." : "The browser can’t decode this video.");
          }}
        />
      </div>
    );
  }

  if (kind === "audio" && url) {
    return (
      <div className="flex items-center gap-4 rounded-2xl bg-muted/60 p-5">
        <span className="grid h-12 w-12 shrink-0 place-items-center rounded-2xl bg-primary text-primary-foreground">
          <Music className="h-5 w-5" />
        </span>
        <audio key={node.id} src={url} controls autoPlay preload="auto" className="w-full" />
      </div>
    );
  }

  if (kind === "pdf" && url) {
    return <iframe title={node.name} src={url} className="h-[62vh] w-full rounded-2xl border bg-white" />;
  }

  if (kind === "text" && text !== null) {
    return (
      <pre className="max-h-[62vh] overflow-auto whitespace-pre-wrap rounded-2xl bg-muted/60 p-5 font-mono text-[13px] leading-relaxed">
        {text}
      </pre>
    );
  }

  return (
    <div className="grid justify-items-center gap-2 rounded-2xl bg-muted/50 px-6 py-12 text-center">
      <p className="font-semibold">No inline preview for this type</p>
      <Button onClick={onDownload}>
        <Download /> Download
      </Button>
    </div>
  );
}

export function PreviewModal({
  token,
  node,
  playlist,
  index,
  onNavigate,
  onClose,
  onDownload,
  onRename,
  onShare,
  onSendTelegram,
  onMediaStudio,
  onEditVideo,
  onDelete,
}: Props) {
  const kind = previewKind(node.name, node.mime_type, false);
  const hasPrev = index > 0;
  const hasNext = index >= 0 && index < playlist.length - 1;
  const counter = playlist.length > 1 && index >= 0 ? `${index + 1} / ${playlist.length}` : null;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      const typing = tag === "INPUT" || tag === "TEXTAREA";
      if (e.key === "Escape") onClose();
      if (typing || playlist.length < 2) return;
      if (e.key === "ArrowLeft" && hasPrev) {
        e.preventDefault();
        onNavigate(index - 1);
      }
      if (e.key === "ArrowRight" && hasNext) {
        e.preventDefault();
        onNavigate(index + 1);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, onNavigate, index, hasPrev, hasNext, playlist.length]);

  const actions = [
    { id: "rename", icon: PenLine, tip: "Rename", run: onRename },
    { id: "share", icon: Link2, tip: "Share", run: onShare },
    { id: "send", icon: Send, tip: "Send to Telegram", run: onSendTelegram },
    ...(onMediaStudio ? [{ id: "studio", icon: Film, tip: "Media Studio", run: onMediaStudio }] : []),
    ...((kind === "video" || kind === "image") && onEditVideo
      ? [{ id: "edit", icon: Scissors, tip: "Edit media", run: onEditVideo }]
      : []),
    { id: "download", icon: Download, tip: "Download", run: onDownload },
  ];

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[92vh] overflow-auto sm:max-w-[880px]">
        <DialogHeader className="flex-row items-start justify-between gap-3 space-y-0">
          <div className="grid min-w-0 gap-1">
            <Badge variant="muted" className="w-fit capitalize">
              {kindLabel(kind)}
            </Badge>
            <DialogTitle className="break-words leading-snug">{node.name}</DialogTitle>
            <p className="text-xs text-muted-foreground">
              {formatBytes(node.size)}
              {counter ? ` · ${counter}` : ""}
            </p>
          </div>
          <TooltipProvider>
            <div className="flex shrink-0 items-center gap-0.5">
              {actions.map((a) => (
                <Tooltip key={a.id}>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="icon-sm" onClick={a.run} aria-label={a.tip}>
                      <a.icon className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{a.tip}</TooltipContent>
                </Tooltip>
              ))}
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon-sm" onClick={onDelete} aria-label="Move to trash" className="text-destructive hover:text-destructive">
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Move to trash</TooltipContent>
              </Tooltip>
            </div>
          </TooltipProvider>
        </DialogHeader>

        {playlist.length > 1 && (
          <div className="flex items-center justify-between gap-2 rounded-xl bg-muted/50 px-2 py-1.5">
            <Button variant="ghost" size="sm" disabled={!hasPrev} onClick={() => onNavigate(index - 1)}>
              <ChevronLeft /> Prev
            </Button>
            {counter && <span className="text-xs text-muted-foreground tabular-nums">{counter}</span>}
            <Button variant="ghost" size="sm" disabled={!hasNext} onClick={() => onNavigate(index + 1)}>
              Next <ChevronRight />
            </Button>
          </div>
        )}

        <PreviewContent token={token} node={node} kind={kind} onDownload={onDownload} onMediaStudio={onMediaStudio} />

        {playlist.length > 1 && (
          <div className="flex gap-2 overflow-auto pb-1" aria-label="Files in folder">
            {playlist.map((n, i) => (
              <button
                key={n.id}
                type="button"
                aria-label={n.name}
                aria-current={i === index ? "true" : undefined}
                onClick={() => onNavigate(i)}
                className={cn(
                  "w-16 shrink-0 overflow-hidden rounded-xl border transition-all",
                  i === index ? "border-ring ring-2 ring-ring/30" : "opacity-70 hover:opacity-100",
                )}
              >
                <FileThumb token={token} id={n.id} name={n.name} mime={n.mime_type} isFolder={false} compact />
              </button>
            ))}
          </div>
        )}

        {(kind === "video" || kind === "image") && (
          <Alert className="border-sky-200 bg-sky-50 text-sky-900">
            <AlertDescription>
              Tip: use ← → keys to browse, Media Studio to convert, Edit to trim & polish.
            </AlertDescription>
          </Alert>
        )}
      </DialogContent>
    </Dialog>
  );
}
