import { useEffect, useMemo, useRef, useState } from "react";
import { searchFiles, type Node, type SearchHit, type User } from "../api";
import { formatBytes, isPreviewable, previewKind } from "../lib/files";
import { BrandMark } from "./BrandMark";
import { DriveCard } from "./DriveCard";
import { FileThumb, KindIcon } from "./FileThumb";
import { PreviewModal } from "./PreviewModal";
import { NoteModal } from "./NoteModal";
import { FetchModal } from "./FetchModal";
import { MoveModal } from "./MoveModal";
import { UserBadge } from "./UserBadge";
import { isFileDrag, isNodeDrag, readNodeDragData, setNodeDragData } from "../lib/drag";

type Crumb = { id: string; name: string };
type ViewMode = "grid" | "list";

type Section = "drive" | "trash";

type Props = {
  token: string;
  user: User | null;
  section: Section;
  title: string;
  trail: Crumb[];
  items: Node[];
  trashItems: Node[];
  busy: boolean;
  error: string;
  onSectionChange: (section: Section) => void;
  onCrumb: (index: number) => void;
  onOpenFolder: (n: Node) => void;
  onNavigate: (crumbs: Crumb[]) => void;
  onMkdir: () => void;
  onFetchURL: (
    input: { url: string; mode: "video" | "audio"; maxHeight: number | null },
    onProgress?: (p: { progress: number; message: string }) => void,
  ) => Promise<void>;
  onUpload: (files: FileList | File[]) => void;
  onDownload: (id: string) => void;
  onRename: (id: string, name: string) => void;
  onMoveMany: (ids: string[], parentId: string) => Promise<void>;
  onDelete: (id: string) => void;
  onDeleteMany: (ids: string[]) => void;
  onPurge: (id: string) => void;
  onEmptyTrash: () => void;
  onShare: (id: string, name: string) => void;
  onSendTelegram: (id: string, name: string) => void;
  onLogout: () => void;
  onClearError: () => void;
};

function locationLabel(path: Crumb[]): string {
  if (path.length <= 1) return "My Drive";
  return path
    .slice(0, -1)
    .map((c) => c.name)
    .join(" / ");
}

export function DriveShell(props: Props) {
  const {
    token,
    user,
    section,
    title,
    trail,
    items,
    trashItems,
    busy,
    error,
    onSectionChange,
    onCrumb,
    onOpenFolder,
    onNavigate,
    onMkdir,
    onFetchURL,
    onUpload,
    onDownload,
    onRename,
    onMoveMany,
    onDelete,
    onDeleteMany,
    onPurge,
    onEmptyTrash,
    onShare,
    onSendTelegram,
    onLogout,
    onClearError,
  } = props;
  const trashMode = section === "trash";

  const [view, setView] = useState<ViewMode>("grid");
  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<SearchHit[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [preview, setPreview] = useState<Node | null>(null);
  const [noteOpen, setNoteOpen] = useState(false);
  const [fetchOpen, setFetchOpen] = useState(false);
  const [fetchBusy, setFetchBusy] = useState(false);
  const [fetchProgress, setFetchProgress] = useState(0);
  const [fetchMessage, setFetchMessage] = useState("");
  const searchingMode = query.trim().length >= 2;
  const [dragOver, setDragOver] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [moveIds, setMoveIds] = useState<string[] | null>(null);
  const [crumbDrop, setCrumbDrop] = useState<number | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const dragDepth = useRef(0);
  const onUploadRef = useRef(onUpload);
  onUploadRef.current = onUpload;

  useEffect(() => {
    setPreview((p) => {
      if (!p) return p;
      const fresh = items.find((i) => i.id === p.id) ?? hits?.find((h) => h.id === p.id);
      if (!fresh) return null;
      return fresh.name !== p.name ? fresh : p;
    });
  }, [items, hits]);

  useEffect(() => {
    setSelected((prev) => {
      if (!prev.size) return prev;
      const source = searchingMode ? (hits ?? []) : items;
      const alive = new Set(source.map((i) => i.id));
      const next = new Set([...prev].filter((id) => alive.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [items, hits, searchingMode]);

  useEffect(() => {
    setSelected(new Set());
  }, [trail]);

  useEffect(() => {
    const q = query.trim();
    if (q.length < 2) {
      setHits(null);
      setSearching(false);
      return;
    }
    let cancelled = false;
    setSearching(true);
    const t = window.setTimeout(() => {
      searchFiles(token, q)
        .then((r) => {
          if (!cancelled) setHits(r.items ?? []);
        })
        .catch(() => {
          if (!cancelled) setHits([]);
        })
        .finally(() => {
          if (!cancelled) setSearching(false);
        });
    }, 220);
    return () => {
      cancelled = true;
      window.clearTimeout(t);
    };
  }, [query, token]);

  useEffect(() => {
    function onEnter(e: DragEvent) {
      if (!isFileDrag(e)) return;
      e.preventDefault();
      dragDepth.current += 1;
      setDragOver(true);
    }
    function onOver(e: DragEvent) {
      if (!isFileDrag(e)) return;
      e.preventDefault();
      if (e.dataTransfer) e.dataTransfer.dropEffect = "copy";
    }
    function onLeave(e: DragEvent) {
      if (!isFileDrag(e)) return;
      e.preventDefault();
      dragDepth.current = Math.max(0, dragDepth.current - 1);
      if (dragDepth.current === 0) setDragOver(false);
    }
    function onDrop(e: DragEvent) {
      if (!isFileDrag(e) || isNodeDrag(e)) return;
      e.preventDefault();
      dragDepth.current = 0;
      setDragOver(false);
      if (e.dataTransfer?.files?.length) onUploadRef.current(e.dataTransfer.files);
    }
    window.addEventListener("dragenter", onEnter);
    window.addEventListener("dragover", onOver);
    window.addEventListener("dragleave", onLeave);
    window.addEventListener("drop", onDrop);
    return () => {
      window.removeEventListener("dragenter", onEnter);
      window.removeEventListener("dragover", onOver);
      window.removeEventListener("dragleave", onLeave);
      window.removeEventListener("drop", onDrop);
    };
  }, []);

  const displayed = useMemo(() => {
    if (trashMode) return trashItems;
    if (searchingMode) return hits ?? [];
    return items;
  }, [trashMode, searchingMode, hits, items, trashItems]);

  const previewPlaylist = useMemo(
    () => displayed.filter((n) => n.type === "file" && isPreviewable(n.name, n.mime_type, false)),
    [displayed],
  );

  const previewIndex = preview ? previewPlaylist.findIndex((n) => n.id === preview.id) : -1;

  const pathById = useMemo(() => {
    const m = new Map<string, Crumb[]>();
    if (!hits) return m;
    for (const h of hits) m.set(h.id, h.path);
    return m;
  }, [hits]);

  const selectedCount = selected.size;
  const selectionMode = selectedCount > 0;
  const allFilteredSelected =
    displayed.length > 0 && displayed.every((n) => selected.has(n.id));

  function toggleSelect(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function selectAllFiltered() {
    setSelected(new Set(displayed.map((n) => n.id)));
  }

  function clearSelection() {
    setSelected(new Set());
  }

  function openMove(ids: string[]) {
    const unique = [...new Set(ids.filter(Boolean))];
    if (!unique.length) return;
    setMoveIds(unique);
  }

  async function confirmMove(parentId: string) {
    if (!moveIds?.length) return;
    try {
      await onMoveMany(moveIds, parentId);
      setMoveIds(null);
      clearSelection();
    } catch {
      /* surfaced by parent */
    }
  }

  async function dragMove(ids: string[], parentId: string) {
    const filtered = ids.filter((id) => id && id !== parentId);
    if (!filtered.length) return;
    await onMoveMany(filtered, parentId);
    clearSelection();
  }

  function openNode(n: Node) {
    if (trashMode) {
      if (n.type === "file") setPreview(n);
      return;
    }
    if (searchingMode) {
      const path = pathById.get(n.id);
      if (n.type === "folder") {
        if (path?.length) onNavigate(path);
        else onOpenFolder(n);
        setQuery("");
        setHits(null);
        return;
      }
      if (path && path.length > 1) onNavigate(path.slice(0, -1));
      setPreview(n);
      return;
    }
    if (n.type === "folder") onOpenFolder(n);
    else setPreview(n);
  }

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "a" && displayed.length) {
        const tag = (e.target as HTMLElement)?.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA") return;
        e.preventDefault();
        selectAllFiltered();
      }
      if (e.key === "Escape" && selectedCount) clearSelection();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [displayed, selectedCount]);

  const pageTitle = trashMode ? "Trash" : searchingMode ? "Search results" : title;
  const pageMeta = trashMode
    ? `${displayed.length} item${displayed.length === 1 ? "" : "s"}`
    : searching
    ? "Searching…"
    : busy && !searchingMode
      ? "Updating…"
      : selectionMode
        ? `${selectedCount} selected`
        : searchingMode
          ? `${displayed.length} result${displayed.length === 1 ? "" : "s"}`
          : `${displayed.length} item${displayed.length === 1 ? "" : "s"}`;

  return (
    <div className="drive-app">
      <aside className={`sidebar ${menuOpen ? "open" : ""}`} aria-label="Sidebar">
        <div className="sidebar-top">
          <BrandMark />
          <button
            type="button"
            className="icon-btn mobile-only"
            aria-label="Close menu"
            onClick={() => setMenuOpen(false)}
          >
            <i className="fa-solid fa-xmark" />
          </button>
        </div>
        <nav className="side-nav">
          <button
            type="button"
            className={`nav-item ${section === "drive" ? "active" : ""}`}
            onClick={() => {
              onSectionChange("drive");
              onCrumb(0);
              setQuery("");
              setHits(null);
              setMenuOpen(false);
            }}
          >
            <i className="fa-solid fa-hard-drive" />
            <span>My Drive</span>
          </button>
          <button
            type="button"
            className={`nav-item ${section === "trash" ? "active" : ""}`}
            onClick={() => {
              onSectionChange("trash");
              setQuery("");
              setHits(null);
              setMenuOpen(false);
            }}
          >
            <i className="fa-solid fa-trash" />
            <span>Trash</span>
          </button>
        </nav>
        <div className="side-foot">
          <UserBadge token={token} user={user} />
          <button type="button" className="btn ghost compact" onClick={onLogout}>
            Sign out
          </button>
        </div>
      </aside>
      {menuOpen && (
        <button
          type="button"
          className="sidebar-scrim mobile-only"
          aria-label="Close menu"
          onClick={() => setMenuOpen(false)}
        />
      )}

      <div className="drive-main">
        <header className="topbar">
          <button
            type="button"
            className="icon-btn mobile-only"
            aria-label="Open menu"
            onClick={() => setMenuOpen(true)}
          >
            <i className="fa-solid fa-bars" />
          </button>
          <div className="search-wrap">
            <label className="sr-only" htmlFor="drive-search">
              Search
            </label>
            <i className="fa-solid fa-magnifying-glass search-icon" aria-hidden />
            <input
              id="drive-search"
              className="search"
              placeholder="Search"
              value={query}
              disabled={trashMode}
              onChange={(e) => setQuery(e.target.value)}
            />
            {query && (
              <button
                type="button"
                className="search-clear"
                aria-label="Clear search"
                onClick={() => {
                  setQuery("");
                  setHits(null);
                }}
              >
                <i className="fa-solid fa-xmark" />
              </button>
            )}
          </div>
          <div className="top-actions">
            <div className="segmented" role="group" aria-label="View mode">
              <button
                type="button"
                className={view === "grid" ? "active" : ""}
                aria-label="Grid view"
                onClick={() => setView("grid")}
              >
                <i className="fa-solid fa-border-all" />
                <span className="hide-sm">Grid</span>
              </button>
              <button
                type="button"
                className={view === "list" ? "active" : ""}
                aria-label="List view"
                onClick={() => setView("list")}
              >
                <i className="fa-solid fa-list" />
                <span className="hide-sm">List</span>
              </button>
            </div>
            <button type="button" className="btn ghost" disabled={busy || searchingMode || trashMode} onClick={onMkdir}>
              <i className="fa-solid fa-folder-plus" />
              <span className="hide-sm">New folder</span>
            </button>
            <button
              type="button"
              className="btn ghost"
              disabled={busy || searchingMode || trashMode}
              onClick={() => setNoteOpen(true)}
            >
              <i className="fa-solid fa-note-sticky" />
              <span className="hide-sm">Note</span>
            </button>
            <button
              type="button"
              className="btn ghost"
              disabled={busy || searchingMode || trashMode}
              onClick={() => setFetchOpen(true)}
            >
              <i className="fa-solid fa-cloud-arrow-down" />
              <span className="hide-sm">Fetch</span>
            </button>
            <button
              type="button"
              className="btn-create"
              disabled={busy || searchingMode || trashMode}
              onClick={() => fileInput.current?.click()}
            >
              <span>
                <svg height={24} width={24} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" aria-hidden>
                  <path d="M0 0h24v24H0z" fill="none" />
                  <path d="M11 11V5h2v6h6v2h-6v6h-2v-6H5v-2z" fill="currentColor" />
                </svg>
                Upload
              </span>
            </button>
            <input
              ref={fileInput}
              type="file"
              multiple
              hidden
              onChange={(e) => {
                if (e.target.files?.length) onUpload(e.target.files);
                e.target.value = "";
              }}
            />
          </div>
        </header>

        {!searchingMode && !trashMode && trail.length > 1 && (
          <div className="crumbs" aria-label="Breadcrumb">
            {trail.map((c, i) => (
              <button
                key={c.id}
                type="button"
                className={`crumb ${crumbDrop === i ? "drop-target" : ""}`}
                onClick={() => onCrumb(i)}
                onDragOver={(e) => {
                  if (!isNodeDrag(e.nativeEvent)) return;
                  e.preventDefault();
                  e.dataTransfer.dropEffect = "move";
                  setCrumbDrop(i);
                }}
                onDragLeave={() => setCrumbDrop((v) => (v === i ? null : v))}
                onDrop={(e) => {
                  if (!isNodeDrag(e.nativeEvent)) return;
                  e.preventDefault();
                  setCrumbDrop(null);
                  const ids = readNodeDragData(e.nativeEvent);
                  if (ids.length) void dragMove(ids, c.id);
                }}
              >
                {c.name}
              </button>
            ))}
          </div>
        )}

        <div className="page-head">
          <div>
            <h1 className="page-title">{pageTitle}</h1>
            <p className="meta">{pageMeta}</p>
          </div>
          {trashMode && displayed.length > 0 && (
            <div className="selection-bar" role="toolbar" aria-label="Trash actions">
              <button type="button" className="btn danger-ghost compact" disabled={busy} onClick={onEmptyTrash}>
                <i className="fa-solid fa-trash" /> Empty trash
              </button>
            </div>
          )}
          {selectionMode && !trashMode && (
            <div className="selection-bar" role="toolbar" aria-label="Selection actions">
              <button type="button" className="btn ghost compact" onClick={clearSelection}>
                Clear
              </button>
              <button
                type="button"
                className="btn ghost compact"
                onClick={allFilteredSelected ? clearSelection : selectAllFiltered}
              >
                {allFilteredSelected ? "Deselect all" : "Select all"}
              </button>
              <button
                type="button"
                className="btn ghost compact"
                disabled={busy}
                onClick={() => openMove(Array.from(selected))}
              >
                <i className="fa-solid fa-folder-tree" /> Move ({selectedCount})
              </button>
              <button
                type="button"
                className="btn danger-ghost compact"
                disabled={busy}
                onClick={() => onDeleteMany(Array.from(selected))}
              >
                <i className="fa-solid fa-trash" /> Delete ({selectedCount})
              </button>
            </div>
          )}
        </div>

        {error && (
          <div className="banner error" role="alert">
            <span>{error}</span>
            <button type="button" className="linkish" onClick={onClearError}>
              Dismiss
            </button>
          </div>
        )}

        <section className={`dropzone ${dragOver ? "over" : ""}`}>
          {dragOver && !searchingMode && (
            <div className="drop-overlay" aria-hidden>
              <i className="fa-solid fa-cloud-arrow-up" />
              <strong>Drop files to upload</strong>
            </div>
          )}
          {(busy && items.length === 0 && !searchingMode) ||
          (searching && searchingMode && hits === null) ? (
            <div className="grid-skel" aria-busy="true">
              {Array.from({ length: 8 }).map((_, i) => (
                <div key={i} className="skeleton card-skel" />
              ))}
            </div>
          ) : displayed.length === 0 ? (
            <div className="empty-panel">
              <i
                className={`fa-regular ${searchingMode ? "fa-magnifying-glass" : "fa-folder-open"} empty-ico`}
              />
              <h2>{trashMode ? "Trash is empty" : searchingMode ? "No results" : "This folder is empty"}</h2>
              <p className="meta">
                {trashMode
                  ? "Deleted files and folders appear here."
                  : searchingMode
                  ? `Nothing matched “${query.trim()}”.`
                  : "Upload files or create a folder to get started."}
              </p>
              {!searchingMode && !trashMode && (
                <div className="row gap">
                  <button type="button" className="btn-create" onClick={() => fileInput.current?.click()}>
                    <span>
                      <svg height={24} width={24} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" aria-hidden>
                        <path d="M0 0h24v24H0z" fill="none" />
                        <path d="M11 11V5h2v6h6v2h-6v6h-2v-6H5v-2z" fill="currentColor" />
                      </svg>
                      Upload
                    </span>
                  </button>
                  <button type="button" className="btn ghost" onClick={onMkdir}>
                    New folder
                  </button>
                  <button type="button" className="btn ghost" onClick={() => setNoteOpen(true)}>
                    Quick note
                  </button>
                  <button type="button" className="btn ghost" onClick={() => setFetchOpen(true)}>
                    Fetch link
                  </button>
                </div>
              )}
            </div>
          ) : view === "grid" ? (
            <ul className="file-grid">
              {displayed.map((n) => (
                <li key={n.id}>
                  <DriveCard
                    token={token}
                    node={n}
                    trashMode={trashMode}
                    movable={!searchingMode}
                    dropTarget={!searchingMode}
                    selected={selected.has(n.id)}
                    selectionMode={selectionMode}
                    subtitle={searchingMode ? locationLabel(pathById.get(n.id) ?? []) : undefined}
                    dragIds={selected.has(n.id) && selectionMode ? Array.from(selected) : [n.id]}
                    onOpen={() => openNode(n)}
                    onToggleSelect={() => toggleSelect(n.id)}
                    onRename={() => onRename(n.id, n.name)}
                    onMove={() => openMove(selected.has(n.id) && selectionMode ? Array.from(selected) : [n.id])}
                    onDownload={() => onDownload(n.id)}
                    onDelete={() => onDelete(n.id)}
                    onPurge={() => onPurge(n.id)}
                    onShare={() => onShare(n.id, n.name)}
                    onSendTelegram={() => onSendTelegram(n.id, n.name)}
                    onDragMove={(ids) => void dragMove(ids, n.id)}
                  />
                </li>
              ))}
            </ul>
          ) : (
            <div className="table-scroll">
              <table className="file-table">
                <thead>
                  <tr>
                    <th className="col-check">
                      <input
                        type="checkbox"
                        checked={allFilteredSelected}
                        onChange={() => (allFilteredSelected ? clearSelection() : selectAllFiltered())}
                        aria-label="Select all"
                      />
                    </th>
                    <th>Name</th>
                    {searchingMode && <th>Location</th>}
                    <th>Type</th>
                    <th>Size</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {displayed.map((n) => {
                    const kind = previewKind(n.name, n.mime_type, n.type === "folder");
                    const isSelected = selected.has(n.id);
                    return (
                      <tr
                        key={n.id}
                        className={`${isSelected ? "selected" : ""} ${n.type === "folder" ? "folder-row" : ""}`}
                        draggable={!trashMode && !searchingMode}
                        onDragStart={(e) => {
                          if (trashMode || searchingMode) return;
                          const ids = isSelected && selectionMode ? Array.from(selected) : [n.id];
                          setNodeDragData(e.nativeEvent, ids);
                        }}
                        onDragOver={(e) => {
                          if (trashMode || searchingMode || n.type !== "folder" || !isNodeDrag(e.nativeEvent)) return;
                          e.preventDefault();
                          e.dataTransfer.dropEffect = "move";
                        }}
                        onDrop={(e) => {
                          if (trashMode || searchingMode || n.type !== "folder" || !isNodeDrag(e.nativeEvent)) return;
                          e.preventDefault();
                          const ids = readNodeDragData(e.nativeEvent).filter((id) => id !== n.id);
                          if (ids.length) void dragMove(ids, n.id);
                        }}
                      >
                        <td className="col-check">
                          <input
                            type="checkbox"
                            checked={isSelected}
                            onChange={() => toggleSelect(n.id)}
                            aria-label={`Select ${n.name}`}
                          />
                        </td>
                        <td>
                          <button type="button" className="name-link" onClick={() => openNode(n)}>
                            <FileThumb
                              token={token}
                              id={n.id}
                              name={n.name}
                              mime={n.mime_type}
                              isFolder={n.type === "folder"}
                              compact
                            />
                            <span title={n.name}>{n.name}</span>
                          </button>
                        </td>
                        {searchingMode && (
                          <td className="meta truncate" title={locationLabel(pathById.get(n.id) ?? [])}>
                            {locationLabel(pathById.get(n.id) ?? [])}
                          </td>
                        )}
                        <td className="meta">
                          <span className="type-cell">
                            <KindIcon kind={kind} />
                            {n.type === "folder" ? "Folder" : kind}
                          </span>
                        </td>
                        <td className="meta">{n.type === "folder" ? "—" : formatBytes(n.size)}</td>
                        <td className="row end table-actions">
                          {trashMode ? (
                            <button type="button" className="card-action danger" title="Delete forever" onClick={() => onPurge(n.id)}>
                              <i className="fa-solid fa-trash" />
                            </button>
                          ) : (
                            <>
                              <button type="button" className="card-action" title="Move" onClick={() => openMove(isSelected && selectionMode ? Array.from(selected) : [n.id])}>
                                <i className="fa-solid fa-folder-tree" />
                              </button>
                              <button type="button" className="card-action" title="Rename" onClick={() => onRename(n.id, n.name)}>
                                <i className="fa-solid fa-pen" />
                              </button>
                              {n.type === "file" && (
                                <button type="button" className="card-action" title="Send to Telegram" onClick={() => onSendTelegram(n.id, n.name)}>
                                  <i className="fa-solid fa-paper-plane" />
                                </button>
                              )}
                              {n.type === "file" && (
                                <button type="button" className="card-action" title="Share" onClick={() => onShare(n.id, n.name)}>
                                  <i className="fa-solid fa-link" />
                                </button>
                              )}
                              {n.type === "file" ? (
                                <button type="button" className="card-action" title="Download" onClick={() => onDownload(n.id)}>
                                  <i className="fa-solid fa-download" />
                                </button>
                              ) : (
                                <span className="card-action spacer" aria-hidden />
                              )}
                              <button type="button" className="card-action danger" title="Move to trash" onClick={() => onDelete(n.id)}>
                                <i className="fa-solid fa-trash" />
                              </button>
                            </>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>

      <nav className="mobile-dock mobile-only" aria-label="Quick actions">
        <button type="button" onClick={() => onCrumb(0)}>
          <i className="fa-solid fa-hard-drive" />
          <span>Drive</span>
        </button>
        <button type="button" onClick={() => onSectionChange("trash")}>
          <i className="fa-solid fa-trash" />
          <span>Trash</span>
        </button>
        <button type="button" onClick={onMkdir}>
          <i className="fa-solid fa-folder-plus" />
          <span>Folder</span>
        </button>
        <button type="button" onClick={() => setNoteOpen(true)}>
          <i className="fa-solid fa-note-sticky" />
          <span>Note</span>
        </button>
        <button type="button" onClick={() => setFetchOpen(true)}>
          <i className="fa-solid fa-cloud-arrow-down" />
          <span>Fetch</span>
        </button>
        <button type="button" className="dock-primary" onClick={() => fileInput.current?.click()}>
          <svg height={22} width={22} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" aria-hidden>
            <path d="M0 0h24v24H0z" fill="none" />
            <path d="M11 11V5h2v6h6v2h-6v6h-2v-6H5v-2z" fill="currentColor" />
          </svg>
          <span>Upload</span>
        </button>
        <button type="button" onClick={onLogout}>
          <i className="fa-solid fa-right-from-bracket" />
          <span>Out</span>
        </button>
      </nav>

      {moveIds && (
        <MoveModal
          token={token}
          busy={busy}
          count={moveIds.length}
          excludeIds={moveIds}
          onClose={() => setMoveIds(null)}
          onConfirm={(parentId) => void confirmMove(parentId)}
        />
      )}

      {noteOpen && (
        <NoteModal
          busy={busy}
          onClose={() => setNoteOpen(false)}
          onSave={(name, body) => {
            const file = new File([body], name, { type: "text/plain;charset=utf-8" });
            setNoteOpen(false);
            onUpload([file]);
          }}
        />
      )}

      {fetchOpen && (
        <FetchModal
          busy={fetchBusy}
          progress={fetchProgress}
          message={fetchMessage}
          onClose={() => {
            if (!fetchBusy) setFetchOpen(false);
          }}
          onFetch={async (input) => {
            setFetchBusy(true);
            setFetchProgress(0);
            setFetchMessage("Starting…");
            try {
              await onFetchURL(input, ({ progress, message }) => {
                setFetchProgress(progress);
                setFetchMessage(message);
              });
              setFetchOpen(false);
            } catch (e) {
              setFetchMessage((e as Error).message);
            } finally {
              setFetchBusy(false);
            }
          }}
        />
      )}

      {preview && (
        <PreviewModal
          token={token}
          node={preview}
          playlist={previewPlaylist}
          index={previewIndex}
          onNavigate={(i) => setPreview(previewPlaylist[i] ?? null)}
          onClose={() => setPreview(null)}
          onDownload={() => onDownload(preview.id)}
          onRename={() => onRename(preview.id, preview.name)}
          onShare={() => onShare(preview.id, preview.name)}
          onSendTelegram={() => onSendTelegram(preview.id, preview.name)}
          onDelete={() => {
            onDelete(preview.id);
            setPreview(null);
          }}
        />
      )}
    </div>
  );
}
