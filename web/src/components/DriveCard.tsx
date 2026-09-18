import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { Node as DriveNode } from "../api";
import { isNodeDrag, readNodeDragData, setNodeDragData } from "../lib/drag";
import { previewKind } from "../lib/files";
import { FileThumb, KindIcon } from "./FileThumb";
import { Portal } from "./Portal";

type Props = {
  token: string;
  node: DriveNode;
  trashMode?: boolean;
  movable?: boolean;
  dropTarget?: boolean;
  selected: boolean;
  selectionMode: boolean;
  subtitle?: string;
  onOpen: () => void;
  onToggleSelect: () => void;
  onRename: () => void;
  onMove: () => void;
  onDownload: () => void;
  onDelete: () => void;
  onPurge: () => void;
  onShare: () => void;
  onSendTelegram: () => void;
  onMediaStudio?: () => void;
  onEditVideo?: () => void;
  onDragMove?: (ids: string[]) => void;
  dragIds?: string[];
};

type MenuPos = { top: number; left: number; openUp: boolean };

export function DriveCard({
  token,
  node,
  trashMode = false,
  movable = false,
  dropTarget = false,
  selected,
  selectionMode,
  subtitle,
  onOpen,
  onToggleSelect,
  onRename,
  onMove,
  onDownload,
  onDelete,
  onPurge,
  onShare,
  onSendTelegram,
  onMediaStudio,
  onEditVideo,
  onDragMove,
  dragIds,
}: Props) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [menuPos, setMenuPos] = useState<MenuPos | null>(null);
  const [dropOver, setDropOver] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);
  const moreRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const kind = previewKind(node.name, node.mime_type, node.type === "folder");

  const canDrag = movable && !trashMode;
  const canDrop = dropTarget && node.type === "folder" && !trashMode;

  function placeMenu() {
    const btn = moreRef.current;
    if (!btn) return;
    const r = btn.getBoundingClientRect();
    const menuW = 200;
    const gap = 6;
    const spaceBelow = window.innerHeight - r.bottom;
    const openUp = spaceBelow < 280 && r.top > spaceBelow;
    let left = r.right - menuW;
    left = Math.max(8, Math.min(left, window.innerWidth - menuW - 8));
    const top = openUp ? r.top - gap : r.bottom + gap;
    setMenuPos({ top, left, openUp });
  }

  useLayoutEffect(() => {
    if (!menuOpen) {
      setMenuPos(null);
      return;
    }
    placeMenu();
  }, [menuOpen]);

  useEffect(() => {
    if (!menuOpen) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setMenuOpen(false);
    }
    function onReposition() {
      placeMenu();
    }
    window.addEventListener("keydown", onKey);
    window.addEventListener("resize", onReposition);
    window.addEventListener("scroll", onReposition, true);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("resize", onReposition);
      window.removeEventListener("scroll", onReposition, true);
    };
  }, [menuOpen]);

  function run(action: () => void) {
    setMenuOpen(false);
    action();
  }

  return (
    <article
      className={`drive-card ${selected ? "selected" : ""} ${dropOver ? "drop-target" : ""} ${menuOpen ? "menu-open" : ""}`}
      ref={wrapRef}
      draggable={canDrag && !menuOpen}
      onDragStart={(e) => {
        if (!canDrag || menuOpen) return;
        const ids = dragIds?.length ? dragIds : [node.id];
        setNodeDragData(e.nativeEvent, ids);
      }}
      onDragEnd={() => setDropOver(false)}
      onDragOver={(e) => {
        if (!canDrop || !isNodeDrag(e.nativeEvent)) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        setDropOver(true);
      }}
      onDragLeave={() => setDropOver(false)}
      onDrop={(e) => {
        if (!canDrop || !isNodeDrag(e.nativeEvent)) return;
        e.preventDefault();
        e.stopPropagation();
        setDropOver(false);
        const ids = readNodeDragData(e.nativeEvent).filter((id) => id !== node.id);
        if (ids.length) onDragMove?.(ids);
      }}
    >
      <div className="drive-card-head">
        <label
          className={`drive-check ${selectionMode || selected ? "visible" : ""}`}
          onClick={(e) => e.stopPropagation()}
        >
          <input
            type="checkbox"
            checked={selected}
            onChange={onToggleSelect}
            aria-label={`Select ${node.name}`}
          />
        </label>
        <button type="button" className="drive-card-title" onClick={onOpen} title={node.name}>
          <KindIcon kind={kind} />
          <span className="drive-card-title-text">
            <span className="truncate">{node.name}</span>
            {subtitle ? <span className="drive-card-sub truncate">{subtitle}</span> : null}
          </span>
        </button>
        <button
          ref={moreRef}
          type="button"
          className={`drive-card-more ${menuOpen ? "open" : ""}`}
          aria-label={`Actions for ${node.name}`}
          aria-expanded={menuOpen}
          aria-haspopup="menu"
          onClick={(e) => {
            e.stopPropagation();
            setMenuOpen((v) => !v);
          }}
        >
          <i className="fa-solid fa-ellipsis-vertical" />
        </button>
      </div>
      <button
        type="button"
        className="drive-card-body"
        onClick={(e) => {
          if (e.metaKey || e.ctrlKey || selectionMode) {
            onToggleSelect();
            return;
          }
          onOpen();
        }}
        aria-label={selectionMode ? `Select ${node.name}` : `Open ${node.name}`}
      >
        {!trashMode && (kind === "video" || kind === "image") && onEditVideo && (
          <span
            className="drive-video-edit-badge"
            role="button"
            tabIndex={0}
            onClick={(e) => {
              e.stopPropagation();
              onEditVideo();
            }}
            onKeyDown={(e) => {
              if (e.key !== "Enter" && e.key !== " ") return;
              e.preventDefault();
              e.stopPropagation();
              onEditVideo();
            }}
          >
            <i className="fa-solid fa-scissors" aria-hidden /> Edit Media
          </span>
        )}
        <FileThumb
          token={token}
          id={node.id}
          name={node.name}
          mime={node.mime_type}
          isFolder={node.type === "folder"}
        />
      </button>

      {menuOpen && menuPos && (
        <Portal>
          <div className="drive-card-menu-layer">
            <button
              type="button"
              className="drive-card-menu-scrim"
              aria-label="Close menu"
              onClick={() => setMenuOpen(false)}
            />
            <div
              ref={menuRef}
              className={`drive-card-menu ${menuPos.openUp ? "open-up" : ""}`}
              role="menu"
              style={{ top: menuPos.top, left: menuPos.left }}
            >
              {!trashMode && (
                <button type="button" role="menuitem" onClick={() => run(onOpen)}>
                  <i className="fa-solid fa-arrow-up-right-from-square" /> Open
                </button>
              )}
              {!trashMode && node.type === "file" && (
                <button type="button" role="menuitem" onClick={() => run(onDownload)}>
                  <i className="fa-solid fa-download" /> Download
                </button>
              )}
              {!trashMode && node.type === "file" && (
                <button type="button" role="menuitem" onClick={() => run(onShare)}>
                  <i className="fa-solid fa-link" /> Share link
                </button>
              )}
              {!trashMode && node.type === "file" && (
                <button type="button" role="menuitem" onClick={() => run(onSendTelegram)}>
                  <i className="fa-solid fa-paper-plane" /> Send to Telegram
                </button>
              )}
              {!trashMode && node.type === "file" && onMediaStudio && (
                <button type="button" role="menuitem" onClick={() => run(onMediaStudio)}>
                  <i className="fa-solid fa-film" /> Media Studio
                </button>
              )}
              {!trashMode && (kind === "video" || kind === "image") && onEditVideo && (
                <button type="button" role="menuitem" onClick={() => run(onEditVideo)}>
                  <i className="fa-solid fa-scissors" /> Edit Media
                </button>
              )}
              {!trashMode && (
                <button type="button" role="menuitem" onClick={() => run(onMove)}>
                  <i className="fa-solid fa-folder-tree" /> Move to…
                </button>
              )}
              {!trashMode && (
                <button type="button" role="menuitem" onClick={() => run(onRename)}>
                  <i className="fa-solid fa-pen" /> Rename
                </button>
              )}
              <div className="drive-card-menu-sep" />
              {trashMode ? (
                <button type="button" role="menuitem" className="danger" onClick={() => run(onPurge)}>
                  <i className="fa-solid fa-trash" /> Delete forever
                </button>
              ) : (
                <button type="button" role="menuitem" className="danger" onClick={() => run(onDelete)}>
                  <i className="fa-solid fa-trash" /> Move to trash
                </button>
              )}
            </div>
          </div>
        </Portal>
      )}
    </article>
  );
}
