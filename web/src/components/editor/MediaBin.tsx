import { useEffect, useMemo, useState } from "react";
import { listFiles, mediaJobStatus, searchFiles, startMediaJob, type MediaJobStatus, type Node } from "../../api";
import { formatBytes, previewKind, type PreviewKind } from "../../lib/files";
import { FileThumb } from "../FileThumb";

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

  return (
    <aside className="media-bin">
      <div className="editor-panel-head">
        <div>
          <p className="editor-panel-title">Media Folder</p>
          <p className="editor-muted">Click + to add. Use music icon on videos to extract audio.</p>
        </div>
        <span className="editor-panel-count">{visibleItems.length}</span>
      </div>
      <div className="media-folder-toolbar">
        <label className="media-search">
          <i className="fa-solid fa-magnifying-glass" />
          <input value={query} placeholder={scope === "drive" ? "Search all Drive..." : "Search this folder..."} onChange={(e) => setQuery(e.target.value)} />
        </label>
        <div className="media-view-toggle" aria-label="Media view mode">
          <button type="button" className={viewMode === "list" ? "active" : ""} onClick={() => setViewMode("list")} title="List view">
            <i className="fa-solid fa-list" />
          </button>
          <button type="button" className={viewMode === "grid" ? "active" : ""} onClick={() => setViewMode("grid")} title="Grid view">
            <i className="fa-solid fa-grip" />
          </button>
        </div>
      </div>
      <div className="media-scope-toggle" aria-label="Media browse scope">
        <button type="button" className={scope === "folder" ? "active" : ""} onClick={() => setScope("folder")}>This folder</button>
        <button type="button" className={scope === "drive" ? "active" : ""} onClick={() => setScope("drive")}>All Drive</button>
      </div>
      <div className="media-filter-tabs" aria-label="Filter media by type">
        <button type="button" className={typeFilter === "all" ? "active" : ""} onClick={() => setTypeFilter("all")}>All <span>{items.length}</span></button>
        <button type="button" className={typeFilter === "video" ? "active" : ""} onClick={() => setTypeFilter("video")}>Video <span>{counts.video}</span></button>
        <button type="button" className={typeFilter === "image" ? "active" : ""} onClick={() => setTypeFilter("image")}>Image <span>{counts.image}</span></button>
        <button type="button" className={typeFilter === "audio" ? "active" : ""} onClick={() => setTypeFilter("audio")}>Audio <span>{counts.audio}</span></button>
      </div>
      {error && <p className="error-text">{error}</p>}
      <div className={`media-bin-list ${viewMode}`}>
        {visibleItems.map((item) => {
          const kind = previewKind(item.name, item.mime_type, false);
          return (
            <button key={item.id} type="button" className="media-bin-item" onClick={() => onAdd(item)}>
              <FileThumb token={token} id={item.id} name={item.name} mime={item.mime_type} isFolder={false} compact />
              <span className="media-bin-meta">
                <strong className="truncate">{item.name}</strong>
                <small><span className={`media-type-chip ${kind}`}>{kindLabel(kind)}</span>{formatBytes(item.size)}</small>
              </span>
              <span className="media-bin-actions">
                {kind === "video" && (
                  <span
                    className="media-extract-badge"
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
                    <i className="fa-solid fa-music" />
                  </span>
                )}
                <span className="media-add-badge" title="Add to timeline"><i className="fa-solid fa-plus" /></span>
              </span>
            </button>
          );
        })}
        {!visibleItems.length && !error && (
          <div className="media-bin-empty">
            <i className="fa-solid fa-photo-film" />
            <span>{items.length ? "No media matches this filter." : "No usable media in this folder yet."}</span>
          </div>
        )}
      </div>
      {Object.values(jobs).length > 0 && (
        <div className="media-job-list" aria-live="polite">
          {Object.values(jobs).map((job) => (
            <div key={job.id} className={`media-job-row ${job.status}`}>
              <i className="fa-solid fa-music" />
              <span className="truncate">{job.status === "done" ? `Saved ${job.node?.name || "audio"}` : job.message || job.phase}</span>
              <small>{Math.round(job.progress)}%</small>
            </div>
          ))}
        </div>
      )}
    </aside>
  );
}

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
