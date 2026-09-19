import { useEffect, useMemo, useRef, useState } from "react";
import {
  Boxes,
  ChevronRight,
  CloudDownload,
  CloudUpload,
  Download,
  Ellipsis,
  Film,
  FolderInput,
  FolderOpen,
  FolderPlus,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  LayoutGrid,
  Link2,
  List as ListIcon,
  LogOut,
  Menu,
  Move,
  PanelLeftClose,
  PanelLeftOpen,
  PenLine,
  Plus,
  Scissors,
  Search,
  SearchX,
  Send,
  Settings2,
  StickyNote,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import { searchFiles, type Node, type SearchHit, type User } from "../api";
import { formatBytes, isPreviewable, previewKind } from "../lib/files";
import { DashboardPage } from "./DashboardPage";
import { HeaderLeadingContext } from "./PageHeader";
import { cn } from "@/lib/utils";
import { BrandMark } from "./BrandMark";
import { DriveCard } from "./DriveCard";
import { FileThumb, KindIcon } from "./FileThumb";
import { PreviewModal } from "./PreviewModal";
import { NoteModal } from "./NoteModal";
import { FetchModal } from "./FetchModal";
import { MoveModal } from "./MoveModal";
import { MediaStudioModal } from "./MediaStudioModal";
import { VideoEditor } from "./editor/VideoEditor";
import { UserMenu } from "./UserMenu";
import { SettingsPage } from "./SettingsPage";
import { S3BucketsPage } from "./S3BucketsPage";
import { isFileDrag, isNodeDrag, readNodeDragData, setNodeDragData } from "../lib/drag";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Checkbox } from "./ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./ui/dropdown-menu";
import { Input } from "./ui/input";
import { Separator } from "./ui/separator";
import { Skeleton } from "./ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "./ui/table";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./ui/tooltip";
import { Sheet, SheetContent } from "./ui/sheet";

type Crumb = { id: string; name: string };
type ViewMode = "grid" | "list";

type Section = "drive" | "trash" | "settings" | "buckets" | "dashboard";

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
    input: { urls: string[]; mode: "video" | "audio"; maxHeight: number | null },
    onProgress?: (p: { progress: number; message: string }) => void,
  ) => Promise<void>;
  onImportURL: (urls: string[]) => Promise<void>;
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
  onRefresh: () => void | Promise<void>;
  onLogout: () => void;
  onClearError: () => void;
  sharedFetchUrl?: string | null;
  onSharedFetchConsumed?: () => void;
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
    onImportURL,
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
    onRefresh,
    onLogout,
    onClearError,
    sharedFetchUrl,
    onSharedFetchConsumed,
  } = props;
  const trashMode = section === "trash";

  const [view, setView] = useState<ViewMode>("list");
  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<SearchHit[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [preview, setPreview] = useState<Node | null>(null);
  const [noteOpen, setNoteOpen] = useState(false);
  const [fetchOpen, setFetchOpen] = useState(false);
  const [fetchBusy, setFetchBusy] = useState(false);
  const [fetchProgress, setFetchProgress] = useState(0);
  const [fetchMessage, setFetchMessage] = useState("");
  const [dockSheet, setDockSheet] = useState<null | "create" | "more">(null);
  const [dockHidden, setDockHidden] = useState(false);
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("nimbus_sidebar") === "collapsed");
  const [mediaNode, setMediaNode] = useState<Node | null>(null);
  const [editVideoNode, setEditVideoNode] = useState<Node | null>(null);
  const [pendingFetchUrl, setPendingFetchUrl] = useState("");
  const lastScrollY = useRef(0);
  const searchingMode = query.trim().length >= 2;

  useEffect(() => {
    if (!sharedFetchUrl?.trim()) return;
    setPendingFetchUrl(sharedFetchUrl.trim());
    setFetchOpen(true);
    setDockSheet(null);
    onSharedFetchConsumed?.();
  }, [sharedFetchUrl, onSharedFetchConsumed]);
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
    if (!dockSheet) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setDockSheet(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [dockSheet]);

  useEffect(() => {
    if (dockSheet) setDockHidden(false);
  }, [dockSheet]);

  useEffect(() => {
    lastScrollY.current = window.scrollY || document.documentElement.scrollTop || 0;
    const onScroll = () => {
      const y = window.scrollY || document.documentElement.scrollTop || 0;
      const delta = y - lastScrollY.current;
      lastScrollY.current = y;
      if (dockSheet) {
        setDockHidden(false);
        return;
      }
      if (y < 40) {
        setDockHidden(false);
        return;
      }
      if (delta > 10) setDockHidden(true);
      else if (delta < -10) setDockHidden(false);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, [dockSheet]);

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

  function goDrive() {
    onSectionChange("drive");
    onCrumb(0);
    setQuery("");
    setHits(null);
    setMenuOpen(false);
  }

  function goTrash() {
    onSectionChange("trash");
    setQuery("");
    setHits(null);
    setMenuOpen(false);
  }

  const toggleSidebar = () =>
    setCollapsed((c) => {
      localStorage.setItem("nimbus_sidebar", c ? "expanded" : "collapsed");
      return !c;
    });
  const sidebarToggle = (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
      title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
      className="hidden -ml-1.5 text-muted-foreground hover:text-foreground lg:inline-flex"
      onClick={toggleSidebar}
    >
      {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
    </Button>
  );
  const navGroups = [
    {
      label: "Files",
      items: [
        { id: "dashboard", label: "Dashboard", icon: LayoutDashboard, active: section === "dashboard", run: () => { onSectionChange("dashboard"); setQuery(""); setHits(null); setMenuOpen(false); } },
        { id: "drive", label: "My Drive", icon: HardDrive, active: section === "drive", run: goDrive },
        { id: "trash", label: "Trash", icon: Trash2, active: section === "trash", run: goTrash },
      ],
    },
    {
      label: "Storage",
      items: [
        { id: "api", label: "Storage API", icon: KeyRound, active: section === "settings", run: () => { onSectionChange("settings"); setQuery(""); setHits(null); setMenuOpen(false); } },
        { id: "s3", label: "S3 Buckets", icon: Boxes, active: section === "buckets", run: () => { onSectionChange("buckets"); setQuery(""); setHits(null); setMenuOpen(false); } },
      ],
    },
  ];

  // Shared sidebar markup: desktop honors collapse (lg-scoped), mobile is always expanded.
  const renderSidebar = (collapsible: boolean) => (
    <>
      <div className={cn("flex items-center px-1.5 py-1 pr-10", collapsible && collapsed && "lg:justify-center lg:pr-1.5")}>
        <div className={cn(collapsible && collapsed && "lg:[&_span]:hidden")}>
          <BrandMark />
        </div>
      </div>
      {navGroups.map((group) => (
        <nav key={group.label} className="grid gap-0.5" aria-label={group.label}>
          <p className={cn(
            "px-3 pb-1 pt-2 text-xs font-medium text-muted-foreground",
            collapsible && collapsed && "lg:hidden",
          )}>
            {group.label}
          </p>
          {group.items.map((item) => (
            <Tooltip key={item.id}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  onClick={item.run}
                  aria-current={item.active ? "page" : undefined}
                  aria-label={item.label}
                  className={cn(
                    "flex items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm font-medium transition-colors",
                    collapsible && collapsed && "lg:gap-0 lg:justify-center lg:px-0 lg:py-2.5",
                    item.active
                      ? "bg-muted text-foreground"
                      : "text-muted-foreground hover:bg-muted/70 hover:text-foreground",
                  )}
                >
                  <item.icon className="h-4 w-4 shrink-0" />
                  <span className={cn(collapsible && collapsed && "lg:hidden")}>{item.label}</span>
                </button>
              </TooltipTrigger>
              {collapsible && collapsed && <TooltipContent side="right">{item.label}</TooltipContent>}
            </Tooltip>
          ))}
        </nav>
      ))}
      <div className="mt-auto grid gap-2 border-t pt-3">
        <UserMenu token={token} user={user} onLogout={onLogout} collapsed={collapsible && collapsed} />
      </div>
    </>
  );
  const sidebarBody = renderSidebar(true);
  const sidebarMobileBody = renderSidebar(false);

  return (
    <TooltipProvider>
      <div
        className={cn(
          "grid min-h-full bg-background transition-[grid-template-columns] duration-200",
          collapsed ? "lg:grid-cols-[72px_minmax(0,1fr)]" : "lg:grid-cols-[260px_minmax(0,1fr)]",
        )}
      >
        {/* Sidebar (desktop) */}
        <aside
          aria-label="Sidebar"
          className={cn(
            "hidden min-w-0 flex-col gap-2 border-r bg-card p-3 lg:sticky lg:top-0 lg:flex lg:h-dvh lg:overflow-y-auto lg:overflow-x-hidden",
            collapsed && "lg:w-[72px]",
          )}
        >
          {sidebarBody}
        </aside>

        {/* Sidebar (mobile) — shadcn Sheet */}
        <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
          <SheetContent side="left" aria-label="Sidebar" className="lg:hidden">
            {sidebarMobileBody}
          </SheetContent>
        </Sheet>

        {/* Main */}
        <div className="flex min-w-0 flex-col px-4 pb-28 pt-4 sm:px-6 lg:pb-10">
          <div className="mb-3 flex items-center gap-2 lg:hidden">
            <Button variant="outline" size="icon" aria-label="Open menu" onClick={() => setMenuOpen((o) => !o)}>
              <Menu className="h-4 w-4" />
            </Button>
            <BrandMark />
          </div>
          <HeaderLeadingContext.Provider value={sidebarToggle}>
          {section === "settings" ? (
            <SettingsPage token={token} />
          ) : section === "buckets" ? (
            <S3BucketsPage token={token} />
          ) : section === "dashboard" ? (
            <DashboardPage token={token} />
          ) : (
          <>
          <header className="mb-3 hidden flex-wrap items-center gap-2.5 lg:flex">
            {sidebarToggle}
            <div className="relative min-w-0 max-w-[540px] flex-1">
              <Search className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id="drive-search"
                placeholder={trashMode ? "Trash isn’t searchable" : "Search your cozy drive…"}
                value={query}
                disabled={trashMode}
                onChange={(e) => setQuery(e.target.value)}
                className="rounded-full bg-card pl-10 pr-10 shadow-xs"
              />
              {query && (
                <button
                  type="button"
                  aria-label="Clear search"
                  onClick={() => {
                    setQuery("");
                    setHits(null);
                  }}
                  className="absolute right-1.5 top-1/2 grid h-7 w-7 -translate-y-1/2 place-items-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              <div className="inline-flex rounded-full border bg-card p-0.5 shadow-xs" role="group" aria-label="View mode">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      aria-label="Grid view"
                      onClick={() => setView("grid")}
                      className={cn(
                        "flex items-center gap-1.5 rounded-full px-3 py-1.5 text-[13px] font-medium transition-colors",
                        view === "grid" ? "bg-muted shadow-xs" : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <LayoutGrid className="h-3.5 w-3.5" />
                      <span className="hidden sm:inline">Grid</span>
                    </button>
                  </TooltipTrigger>
                  <TooltipContent>Grid view</TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      aria-label="List view"
                      onClick={() => setView("list")}
                      className={cn(
                        "flex items-center gap-1.5 rounded-full px-3 py-1.5 text-[13px] font-medium transition-colors",
                        view === "list" ? "bg-muted shadow-xs" : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <ListIcon className="h-3.5 w-3.5" />
                      <span className="hidden sm:inline">List</span>
                    </button>
                  </TooltipTrigger>
                  <TooltipContent>List view</TooltipContent>
                </Tooltip>
              </div>
              <Button variant="outline" disabled={busy || searchingMode || trashMode} onClick={onMkdir}>
                <FolderPlus />
                <span className="hidden sm:inline">New folder</span>
              </Button>
              <Button variant="outline" disabled={busy || searchingMode || trashMode} onClick={() => setNoteOpen(true)}>
                <StickyNote />
                <span className="hidden sm:inline">Note</span>
              </Button>
              <Button variant="outline" disabled={busy || searchingMode || trashMode} onClick={() => setFetchOpen(true)}>
                <CloudDownload />
                <span className="hidden sm:inline">Fetch</span>
              </Button>
              <Button disabled={busy || searchingMode || trashMode} onClick={() => fileInput.current?.click()}>
                <Upload /> Upload
              </Button>
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
            <nav className="mb-2 flex max-w-full items-center gap-0.5 overflow-x-auto pb-1" aria-label="Breadcrumb">
              {trail.map((c, i) => (
                <span key={c.id} className="flex shrink-0 items-center gap-0.5">
                  {i > 0 && <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />}
                  <button
                    type="button"
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
                    className={cn(
                      "rounded-lg px-2 py-1 text-sm font-medium transition-colors",
                      i === trail.length - 1 ? "text-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                      crumbDrop === i && "bg-sky-100 text-sky-700",
                    )}
                  >
                    {c.name}
                  </button>
                </span>
              ))}
            </nav>
          )}

          <div className="mb-3 flex flex-wrap items-end justify-between gap-2.5">
            <div>
              <h1 className="text-[clamp(1.6rem,2.6vw,2.1rem)] font-semibold tracking-tight">{pageTitle}</h1>
              <p className="text-sm text-muted-foreground">
                {pageMeta}
                {selectionMode && (
                  <Badge variant="secondary" className="ml-2">
                    {selectedCount} picked
                  </Badge>
                )}
              </p>
            </div>
            {trashMode && displayed.length > 0 && (
              <Button variant="destructive" size="sm" disabled={busy} onClick={onEmptyTrash}>
                <Trash2 /> Empty trash
              </Button>
            )}
            {selectionMode && !trashMode && (
              <div className="flex flex-wrap items-center gap-1.5" role="toolbar" aria-label="Selection actions">
                <Button variant="ghost" size="sm" onClick={clearSelection}>
                  Clear
                </Button>
                <Button variant="outline" size="sm" onClick={allFilteredSelected ? clearSelection : selectAllFiltered}>
                  {allFilteredSelected ? "Deselect all" : "Select all"}
                </Button>
                <Button variant="outline" size="sm" disabled={busy} onClick={() => openMove(Array.from(selected))}>
                  <FolderInput /> Move ({selectedCount})
                </Button>
                <Button variant="destructive" size="sm" disabled={busy} onClick={() => onDeleteMany(Array.from(selected))}>
                  <Trash2 /> Delete ({selectedCount})
                </Button>
              </div>
            )}
          </div>

          {error && (
            <Alert variant="destructive" className="mb-3">
              <AlertDescription className="flex w-full items-center justify-between gap-2">
                <span>{error}</span>
                <Button variant="ghost" size="sm" onClick={onClearError} className="shrink-0">
                  Dismiss
                </Button>
              </AlertDescription>
            </Alert>
          )}

          <section className={cn("relative min-h-[280px] flex-1 rounded-2xl transition-colors", dragOver && "bg-sky-50 ring-2 ring-sky-500/40")}>
            {dragOver && !searchingMode && (
              <div className="pointer-events-none absolute inset-0 z-10 grid place-content-center gap-2 rounded-2xl border-2 border-dashed border-sky-500 bg-white/80 text-center">
                <CloudUpload className="mx-auto h-7 w-7 text-sky-600" />
                <strong className="tracking-tight">Drop files to upload</strong>
              </div>
            )}
            {(busy && items.length === 0 && !searchingMode) ||
            (searching && searchingMode && hits === null) ? (
              <div className="grid grid-cols-[repeat(auto-fill,minmax(150px,1fr))] gap-3" aria-busy="true">
                {Array.from({ length: 8 }).map((_, i) => (
                  <div key={i} className="grid gap-2">
                    <Skeleton className="h-[150px] w-full" />
                    <Skeleton className="h-3.5 w-3/4" />
                  </div>
                ))}
              </div>
            ) : displayed.length === 0 ? (
              <Card className="grid gap-2 p-8">
                <span className="grid h-12 w-12 place-items-center rounded-2xl bg-muted text-muted-foreground">
                  {searchingMode ? <SearchX className="h-5 w-5" /> : <FolderOpen className="h-5 w-5" />}
                </span>
                <h2 className="text-lg font-semibold tracking-tight">
                  {trashMode ? "Trash is empty" : searchingMode ? "No results" : "This folder is empty"}
                </h2>
                <p className="text-sm text-muted-foreground">
                  {trashMode
                    ? "Deleted files and folders will rest here. Cozy, right?"
                    : searchingMode
                    ? `Nothing matched “${query.trim()}”. Try another word?`
                    : "Upload files or create a folder to get started — you’ve got this!"}
                </p>
                {!searchingMode && !trashMode && (
                  <div className="mt-2 flex flex-wrap gap-2">
                    <Button onClick={() => fileInput.current?.click()}>
                      <Upload /> Upload
                    </Button>
                    <Button variant="outline" onClick={onMkdir}>
                      New folder
                    </Button>
                    <Button variant="outline" onClick={() => setNoteOpen(true)}>
                      Quick note
                    </Button>
                    <Button variant="outline" onClick={() => setFetchOpen(true)}>
                      Fetch link
                    </Button>
                  </div>
                )}
              </Card>
            ) : view === "grid" ? (
              <ul className="grid grid-cols-[repeat(auto-fill,minmax(150px,1fr))] gap-3">
                {displayed.map((n) => (
                  <li key={n.id} className="min-w-0">
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
                      onMediaStudio={() => setMediaNode(n)}
                      onEditVideo={() => setEditVideoNode(n)}
                      onDragMove={(ids) => void dragMove(ids, n.id)}
                    />
                  </li>
                ))}
              </ul>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">
                      <Checkbox
                        checked={allFilteredSelected}
                        onCheckedChange={() => (allFilteredSelected ? clearSelection() : selectAllFiltered())}
                        aria-label="Select all"
                      />
                    </TableHead>
                    <TableHead>Name</TableHead>
                    {searchingMode && <TableHead>Location</TableHead>}
                    <TableHead>Type</TableHead>
                    <TableHead>Size</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {displayed.map((n) => {
                    const kind = previewKind(n.name, n.mime_type, n.type === "folder");
                    const isSelected = selected.has(n.id);
                    return (
                      <TableRow
                        key={n.id}
                        data-state={isSelected ? "selected" : undefined}
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
                        <TableCell>
                          <Checkbox
                            checked={isSelected}
                            onCheckedChange={() => toggleSelect(n.id)}
                            aria-label={`Select ${n.name}`}
                          />
                        </TableCell>
                        <TableCell>
                          <button type="button" onClick={() => openNode(n)} className="flex max-w-[380px] items-center gap-2.5 text-left">
                            <FileThumb
                              token={token}
                              id={n.id}
                              name={n.name}
                              mime={n.mime_type}
                              isFolder={n.type === "folder"}
                              compact
                            />
                            <span className="truncate text-sm font-medium" title={n.name}>
                              {n.name}
                            </span>
                          </button>
                        </TableCell>
                        {searchingMode && (
                          <TableCell className="max-w-[180px] truncate text-muted-foreground" title={locationLabel(pathById.get(n.id) ?? [])}>
                            {locationLabel(pathById.get(n.id) ?? [])}
                          </TableCell>
                        )}
                        <TableCell>
                          <span className="inline-flex items-center gap-1.5 text-muted-foreground capitalize">
                            <KindIcon kind={kind} />
                            {n.type === "folder" ? "Folder" : kind}
                          </span>
                        </TableCell>
                        <TableCell className="text-muted-foreground">{n.type === "folder" ? "—" : formatBytes(n.size)}</TableCell>
                        <TableCell>
                          <div className="flex justify-end">
                            {trashMode ? (
                              <DropdownMenu>
                                <DropdownMenuTrigger asChild>
                                  <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${n.name}`}>
                                    <Ellipsis className="h-4 w-4" />
                                  </Button>
                                </DropdownMenuTrigger>
                                <DropdownMenuContent align="end">
                                  <DropdownMenuItem className="text-destructive focus:text-destructive" onClick={() => onPurge(n.id)}>
                                    <Trash2 className="h-4 w-4" /> Delete forever
                                  </DropdownMenuItem>
                                </DropdownMenuContent>
                              </DropdownMenu>
                            ) : (
                              <DropdownMenu>
                                <DropdownMenuTrigger asChild>
                                  <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${n.name}`}>
                                    <Ellipsis className="h-4 w-4" />
                                  </Button>
                                </DropdownMenuTrigger>
                                <DropdownMenuContent align="end" className="w-48">
                                  {n.type === "file" && (
                                    <DropdownMenuItem onClick={() => onDownload(n.id)}>
                                      <Download className="h-4 w-4" /> Download
                                    </DropdownMenuItem>
                                  )}
                                  <DropdownMenuItem onClick={() => openMove(isSelected && selectionMode ? Array.from(selected) : [n.id])}>
                                    <Move className="h-4 w-4" /> Move
                                  </DropdownMenuItem>
                                  <DropdownMenuItem onClick={() => onRename(n.id, n.name)}>
                                    <PenLine className="h-4 w-4" /> Rename
                                  </DropdownMenuItem>
                                  {n.type === "file" && (
                                    <>
                                      <DropdownMenuSeparator />
                                      <DropdownMenuItem onClick={() => onShare(n.id, n.name)}>
                                        <Link2 className="h-4 w-4" /> Share
                                      </DropdownMenuItem>
                                      <DropdownMenuItem onClick={() => onSendTelegram(n.id, n.name)}>
                                        <Send className="h-4 w-4" /> Send to Telegram
                                      </DropdownMenuItem>
                                      <DropdownMenuItem onClick={() => setMediaNode(n)}>
                                        <Film className="h-4 w-4" /> Media Studio
                                      </DropdownMenuItem>
                                      {(kind === "video" || kind === "image") && (
                                        <DropdownMenuItem onClick={() => setEditVideoNode(n)}>
                                          <Scissors className="h-4 w-4" /> Edit media
                                        </DropdownMenuItem>
                                      )}
                                    </>
                                  )}
                                  <DropdownMenuSeparator />
                                  <DropdownMenuItem className="text-destructive focus:text-destructive" onClick={() => onDelete(n.id)}>
                                    <Trash2 className="h-4 w-4" /> Move to trash
                                  </DropdownMenuItem>
                                </DropdownMenuContent>
                              </DropdownMenu>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}
          </section>

          <div className="mt-3 hidden items-center gap-2 text-xs text-muted-foreground lg:flex">
            <Settings2 className="h-3.5 w-3.5" />
            Tip: drag files anywhere to upload · press Ctrl/⌘+A to select all · ← → to browse previews.
          </div>
          </>
          )}
          </HeaderLeadingContext.Provider>
        </div>

        {/* Mobile dock */}
        <nav
          aria-label="Quick actions"
          className={cn(
            "fixed inset-x-3 bottom-3 z-40 grid grid-cols-3 gap-1 rounded-2xl border bg-card/95 p-1.5 shadow-xl backdrop-blur transition-transform lg:hidden",
            dockHidden && !dockSheet && "translate-y-[120%]",
          )}
        >
          <button
            type="button"
            onClick={() => {
              setDockSheet(null);
              onSectionChange("drive");
              onCrumb(0);
              setQuery("");
              setHits(null);
            }}
            className={cn(
              "grid justify-items-center gap-0.5 rounded-xl px-2 py-2 text-[11px] font-medium",
              section === "drive" && !dockSheet ? "bg-muted" : "text-muted-foreground",
            )}
          >
            <HardDrive className="h-4 w-4" />
            Drive
          </button>
          <button
            type="button"
            aria-expanded={dockSheet === "create"}
            onClick={() => setDockSheet((s) => (s === "create" ? null : "create"))}
            className={cn(
              "grid justify-items-center gap-0.5 rounded-xl px-2 py-2 text-[11px] font-medium",
              dockSheet === "create" ? "bg-primary text-primary-foreground" : "bg-primary/10 text-primary",
            )}
          >
            {dockSheet === "create" ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
            Create
          </button>
          <button
            type="button"
            aria-expanded={dockSheet === "more"}
            onClick={() => setDockSheet((s) => (s === "more" ? null : "more"))}
            className={cn(
              "grid justify-items-center gap-0.5 rounded-xl px-2 py-2 text-[11px] font-medium",
              section === "trash" || dockSheet === "more" ? "bg-muted" : "text-muted-foreground",
            )}
          >
            <Ellipsis className="h-4 w-4" />
            More
          </button>
        </nav>

        {dockSheet && (
          <div className="fixed inset-0 z-50 lg:hidden">
            <button type="button" className="absolute inset-0 bg-black/30" aria-label="Close menu" onClick={() => setDockSheet(null)} />
            <div className="absolute inset-x-3 bottom-[76px] grid gap-1 rounded-2xl border bg-card p-2 shadow-xl" role="menu">
              {dockSheet === "create" ? (
                <>
                  {[
                    { id: "upload", title: "Upload", sub: "Photos, videos, files", icon: CloudUpload, run: () => { fileInput.current?.click(); setDockSheet(null); } },
                    { id: "folder", title: "New folder", sub: "Organize your drive", icon: FolderPlus, run: () => { setDockSheet(null); onMkdir(); } },
                    { id: "note", title: "Note", sub: "Quick text file", icon: StickyNote, run: () => { setDockSheet(null); setNoteOpen(true); } },
                    { id: "fetch", title: "Fetch link", sub: "From the web", icon: CloudDownload, run: () => { setDockSheet(null); setFetchOpen(true); } },
                  ].map((a) => (
                    <button key={a.id} type="button" role="menuitem" onClick={a.run} className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-left hover:bg-muted">
                      <span className="grid h-9 w-9 place-items-center rounded-xl bg-muted">
                        <a.icon className="h-4 w-4" />
                      </span>
                      <span className="grid">
                        <strong className="text-sm">{a.title}</strong>
                        <small className="text-xs text-muted-foreground">{a.sub}</small>
                      </span>
                    </button>
                  ))}
                </>
              ) : (
                <>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setDockSheet(null);
                      onSectionChange("dashboard");
                      setQuery("");
                      setHits(null);
                    }}
                    className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-left hover:bg-muted"
                  >
                    <span className="grid h-9 w-9 place-items-center rounded-xl bg-muted">
                      <LayoutDashboard className="h-4 w-4" />
                    </span>
                    <span className="grid">
                      <strong className="text-sm">Dashboard</strong>
                      <small className="text-xs text-muted-foreground">Storage stats</small>
                    </span>
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setDockSheet(null);
                      onSectionChange("trash");
                      setQuery("");
                      setHits(null);
                    }}
                    className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-left hover:bg-muted"
                  >
                    <span className="grid h-9 w-9 place-items-center rounded-xl bg-muted">
                      <Trash2 className="h-4 w-4" />
                    </span>
                    <span className="grid">
                      <strong className="text-sm">Trash</strong>
                      <small className="text-xs text-muted-foreground">Deleted files</small>
                    </span>
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setDockSheet(null);
                      onSectionChange("buckets");
                      setQuery("");
                      setHits(null);
                    }}
                    className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-left hover:bg-muted"
                  >
                    <span className="grid h-9 w-9 place-items-center rounded-xl bg-muted">
                      <Boxes className="h-4 w-4" />
                    </span>
                    <span className="grid">
                      <strong className="text-sm">S3 Buckets</strong>
                      <small className="text-xs text-muted-foreground">Object storage</small>
                    </span>
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setDockSheet(null);
                      onSectionChange("settings");
                      setQuery("");
                      setHits(null);
                    }}
                    className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-left hover:bg-muted"
                  >
                    <span className="grid h-9 w-9 place-items-center rounded-xl bg-muted">
                      <Settings2 className="h-4 w-4" />
                    </span>
                    <span className="grid">
                      <strong className="text-sm">Settings</strong>
                      <small className="text-xs text-muted-foreground">Storage API & key</small>
                    </span>
                  </button>
                  <Separator />
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setDockSheet(null);
                      onLogout();
                    }}
                    className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-left text-destructive hover:bg-destructive/10"
                  >
                    <span className="grid h-9 w-9 place-items-center rounded-xl bg-destructive/10">
                      <LogOut className="h-4 w-4" />
                    </span>
                    <span className="grid">
                      <strong className="text-sm">Sign out</strong>
                      <small className="text-xs opacity-70">End this session</small>
                    </span>
                  </button>
                </>
              )}
            </div>
          </div>
        )}

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
            initialUrl={pendingFetchUrl}
            onClose={() => {
              if (!fetchBusy) {
                setFetchOpen(false);
                setPendingFetchUrl("");
              }
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
            onImport={async (importUrl) => {
              setFetchBusy(true);
              setFetchProgress(0);
              setFetchMessage("Importing…");
              try {
                await onImportURL(importUrl);
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
            onMediaStudio={() => {
              setMediaNode(preview);
              setPreview(null);
            }}
            onEditVideo={() => {
              setEditVideoNode(preview);
              setPreview(null);
            }}
            onDelete={() => {
              onDelete(preview.id);
              setPreview(null);
            }}
          />
        )}

        {mediaNode && (
          <MediaStudioModal
            token={token}
            node={mediaNode}
            busy={busy}
            onClose={() => setMediaNode(null)}
            onDone={() => {
              void onRefresh();
            }}
          />
        )}

        {editVideoNode && (
          <VideoEditor
            token={token}
            node={editVideoNode}
            onClose={() => setEditVideoNode(null)}
            onDone={() => {
              setEditVideoNode(null);
              void onRefresh();
            }}
          />
        )}
      </div>
    </TooltipProvider>
  );
}
