import { useEffect, useMemo, useState } from "react";
import { Film, LayoutGrid, List, Music, Plus, Search } from "lucide-react";
import { listFiles, mediaJobStatus, searchFiles, startMediaJob, type MediaJobStatus, type Node } from "../../api";
import { formatBytes, previewKind, type PreviewKind } from "../../lib/files";
import { cn } from "@/lib/utils";
import { FileThumb } from "../FileThumb";
import { Alert, AlertDescription } from "../ui/alert";
import { Input } from "../ui/input";

type Props = {
  token: string;
  parentID: string;
  onAdd: (node: Node) => void;
  onChanged?: () => void;
};

export function MediaBin({ token, parentID, onAdd, onChanged }: Props) {
  const [items, setItems] = useState<Node[]>([]);
  const [error, setError] = useState("");
  const [jobs, setJobs] = useState<Record<string, MediaJobStatus>>({});
  const [query, setQuery] = useState("");
  const [scope, setScope] = useState<"folder" | "drive">("folder");
  const [typeFilter, setTypeFilter] = useState<"all" | "video" | "image" | "audio">("all");
  const [viewMode, setViewMode] = useState<"list" | "grid">("list");

  const counts = useMemo(() => {
    return items.reduce(
      (total, item) => {
        const kind = previewKind(item.name, item.mime_type, false);
        if (kind === "video" || kind === "image" || kind === "audio") total[kind] += 1;
        return total;
      },
      { video: 0, image: 0, audio: 0 },
    );
  }, [items]);

  const visibleItems = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return items.filter((item) => {
      const kind = previewKind(item.name, item.mime_type, false);
      if (typeFilter !== "all" && kind !== typeFilter) return false;
      if (!needle) return true;
      return item.name.toLowerCase().includes(needle);
    });
  }, [items, query, typeFilter]);

  useEffect(() => {
    let cancelled = false;
    loadItems(token, parentID, scope, query)
      .then((nextItems) => {
        if (!cancelled) setItems(nextItems);
      })
      .catch((e) => {
        if (!cancelled) setError((e as Error).message);
      });
    return () => {
      cancelled = true;
    };
  }, [parentID, query, scope, token]);

  useEffect(() => {
    const activeJobs = Object.values(jobs).filter((job) => job.status === "queued" || job.status === "running");
    if (!activeJobs.length) return;
    const timer = window.setInterval(() => {
      activeJobs.forEach((job) => {
        mediaJobStatus(token, job.id)
          .then((status) => {
            setJobs((current) => ({ ...current, [job.id]: status }));
            if (status.status === "done") {
              loadItems(token, parentID, scope, query).then(setItems).catch(() => undefined);
              onChanged?.();
            }
          })
          .catch((e) => setError((e as Error).message));
      });
    }, 900);
    return () => window.clearInterval(timer);
  }, [jobs, onChanged, parentID, query, scope, token]);

  async function extractAudio(item: Node) {
    setError("");
    try {
      const { job_id } = await startMediaJob(token, item.id, "extract_audio");
      setJobs((current) => ({
        ...current,
        [job_id]: {
          id: job_id,
          action: "extract_audio",
          status: "queued",
          phase: "queued",
          progress: 0,
          message: "Extracting audio...",
        },
      }));
    } catch (e) {
      setError((e as Error).message);
    }
  }

  const typeTabs: { id: "all" | "video" | "image" | "audio"; label: string; count: number }[] = [
    { id: "all", label: "All", count: items.length },
    { id: "video", label: "Video", count: counts.video },
    { id: "image", label: "Image", count: counts.image },
    { id: "audio", label: "Audio", count: counts.audio },
  ];

  return (
    <div className="grid min-h-0 grid-rows-[auto_auto_auto_auto_minmax(0,1fr)_auto] gap-3 overflow-hidden p-3.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-semibold">Media Folder</p>
          <p className="text-xs text-muted-foreground">Click + to add. Use the music icon to extract audio.</p>
        </div>
        <span className="min-w-7 rounded-full border bg-muted px-2 py-0.5 text-center text-xs font-semibold text-muted-foreground">
          {visibleItems.length}
        </span>
      </div>
      <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-1.5">
        <div className="flex min-h-9 items-center gap-2 rounded-xl border bg-card px-3">
          <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
          <Input
            value={query}
            placeholder={scope === "drive" ? "Search all Drive..." : "Search this folder..."}
            onChange={(e) => setQuery(e.target.value)}
            className="h-8 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0"
          />
        </div>
        <div className="inline-flex items-center gap-0.5 rounded-xl border bg-card p-0.5" aria-label="Media view mode">
          {([["list", List], ["grid", LayoutGrid]] as const).map(([mode, Icon]) => (
            <button
              key={mode}
              type="button"
              onClick={() => setViewMode(mode)}
              title={`${mode === "list" ? "List" : "Grid"} view`}
              className={cn(
                "grid h-7 w-7 place-items-center rounded-lg border-0 bg-transparent text-muted-foreground transition-colors hover:text-foreground",
                viewMode === mode ? "bg-muted text-foreground" : "",
              )}
            >
              <Icon className="h-4 w-4" />
            </button>
          ))}
        </div>
      </div>
      <div className="grid grid-cols-2 gap-1 rounded-xl border bg-muted/40 p-0.5" aria-label="Media browse scope">
        {([["folder", "This folder"], ["drive", "All Drive"]] as const).map(([id, label]) => (
          <button
            key={id}
            type="button"
            onClick={() => setScope(id)}
            className={cn(
              "min-h-8 rounded-lg border-0 text-xs font-semibold transition-colors",
              scope === id ? "bg-card text-foreground shadow-xs" : "bg-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {label}
          </button>
        ))}
      </div>
      <div className="flex gap-1.5 overflow-x-auto pb-0.5" aria-label="Filter media by type">
        {typeTabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => setTypeFilter(tab.id)}
            className={cn(
              "inline-flex min-h-8 items-center gap-1.5 whitespace-nowrap rounded-full border px-3 text-xs font-semibold transition-colors",
              typeFilter === tab.id
                ? "border-primary/30 bg-primary/10 text-foreground"
                : "border-input bg-card text-muted-foreground hover:text-foreground",
            )}
          >
            {tab.label}
            <span className="text-muted-foreground">{tab.count}</span>
          </button>
        ))}
      </div>
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className={cn("grid min-h-0 content-start gap-2 overflow-auto pr-0.5", viewMode === "grid" && "grid-cols-2")}>
        {visibleItems.map((item) => {
          const kind = previewKind(item.name, item.mime_type, false);
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => onAdd(item)}
              className={cn(
                "group relative grid w-full cursor-pointer items-center gap-2.5 rounded-xl border bg-card p-2 text-left transition-colors hover:border-primary/40 hover:bg-muted/60",
                viewMode === "list" ? "grid-cols-[46px_minmax(0,1fr)_auto] min-h-[58px]" : "content-start",
              )}
            >
              <FileThumb token={token} id={item.id} name={item.name} mime={item.mime_type} isFolder={false} compact />
              <span className="grid min-w-0 gap-0.5">
                <strong className="truncate text-sm font-medium">{item.name}</strong>
                <small className="flex items-center gap-1.5 text-xs text-muted-foreground">
                  <span className={cn("inline-flex min-w-12 items-center justify-center rounded-full px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide text-white", KIND_CHIP[kind])}>
                    {kindLabel(kind)}
                  </span>
                  {formatBytes(item.size)}
                </small>
              </span>
              <span className={cn("flex items-center gap-1.5", viewMode === "grid" && "absolute right-2 top-2")}>
                {kind === "video" && (
                  <span
                    className="inline-grid h-7 w-7 cursor-pointer place-items-center rounded-full bg-emerald-100 text-emerald-700 transition-colors hover:bg-emerald-200"
                    role="button"
                    tabIndex={0}
                    title="Extract audio from this video"
                    onClick={(e) => {
                      e.stopPropagation();
                      void extractAudio(item);
                    }}
                    onKeyDown={(e) => {
                      if (e.key !== "Enter" && e.key !== " ") return;
                      e.preventDefault();
                      e.stopPropagation();
                      void extractAudio(item);
                    }}
                  >
                    <Music className="h-3.5 w-3.5" />
                  </span>
                )}
                <span className="inline-grid h-7 w-7 place-items-center rounded-full bg-primary/10 text-primary" title="Add to timeline">
                  <Plus className="h-3.5 w-3.5" />
                </span>
              </span>
            </button>
          );
        })}
        {!visibleItems.length && !error && (
          <div className="col-span-full grid min-h-[140px] place-items-center gap-2 rounded-2xl border border-dashed p-4 text-center text-muted-foreground">
            <Film className="h-4 w-4" />
            <span className="text-sm">{items.length ? "No media matches this filter." : "No usable media in this folder yet."}</span>
          </div>
        )}
      </div>
      {Object.values(jobs).length > 0 && (
        <div className="grid gap-1.5 pt-1" aria-live="polite">
          {Object.values(jobs).map((job) => (
            <div
              key={job.id}
              className={cn(
                "grid grid-cols-[18px_minmax(0,1fr)_auto] items-center gap-2 rounded-xl border bg-muted/40 px-2.5 py-2 text-xs text-muted-foreground",
                job.status === "done" && "text-emerald-600",
              )}
            >
              <Music className="h-3.5 w-3.5" />
              <span className="truncate">{job.status === "done" ? `Saved ${job.node?.name || "audio"}` : job.message || job.phase}</span>
              <small>{Math.round(job.progress)}%</small>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

const KIND_CHIP: Record<PreviewKind, string> = {
  video: "bg-sky-500",
  image: "bg-fuchsia-500",
  audio: "bg-emerald-500",
  folder: "bg-amber-500",
  pdf: "bg-red-500",
  text: "bg-slate-500",
  archive: "bg-slate-500",
  file: "bg-slate-500",
};

async function loadItems(token: string, parentID: string, scope: "folder" | "drive", query: string) {
  const needle = query.trim();
  const items = scope === "drive" && needle
    ? (await searchFiles(token, needle)).items
    : scope === "drive"
      ? (await listFiles(token, "", { recursive: true, limit: 1000 })).items
    : (await listFiles(token, parentID)).items;
  return items.filter((item) => {
    const kind = previewKind(item.name, item.mime_type, item.type === "folder");
    return kind === "video" || kind === "image" || kind === "audio";
  });
}

function kindLabel(kind: PreviewKind) {
  switch (kind) {
    case "video":
      return "Video";
    case "image":
      return "Image";
    case "audio":
      return "Audio";
    default:
      return "File";
  }
}
