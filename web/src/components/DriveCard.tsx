import { useEffect, useRef, useState } from "react";
import type { Node as DriveNode } from "../api";
import { isNodeDrag, readNodeDragData, setNodeDragData } from "../lib/drag";
import { previewKind } from "../lib/files";
import { FileThumb, KindIcon } from "./FileThumb";

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
  onDragMove?: (ids: string[]) => void;
  dragIds?: string[];
};

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
  onDragMove,
  dragIds,
}: Props) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [dropOver, setDropOver] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);
  const kind = previewKind(node.name, node.mime_type, node.type === "folder");

  const canDrag = movable && !trashMode;
  const canDrop = dropTarget && node.type === "folder" && !trashMode;

  useEffect(() => {
    if (!menuOpen) return;
    function onDoc(e: MouseEvent) {
      if (!wrapRef.current?.contains(e.target as Node)) setMenuOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setMenuOpen(false);
    }
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [menuOpen]);

  return (
    <article
      className={`drive-card ${selected ? "selected" : ""} ${dropOver ? "drop-target" : ""}`}
      ref={wrapRef}
      draggable={canDrag}
      onDragStart={(e) => {
        if (!canDrag) return;
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
        {menuOpen && (
          <div className="drive-card-menu" role="menu">
            {!trashMode && (
              <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); onOpen(); }}>
                <i className="fa-solid fa-arrow-up-right-from-square" /> Open
              </button>
            )}
            {!trashMode && node.type === "file" && (
              <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); onDownload(); }}>
                <i className="fa-solid fa-download" /> Download
              </button>
            )}
            {!trashMode && node.type === "file" && (
              <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); onShare(); }}>
                <i className="fa-solid fa-link" /> Share link
              </button>
            )}
            {!trashMode && node.type === "file" && (
              <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); onSendTelegram(); }}>
                <i className="fa-solid fa-paper-plane" /> Send to Telegram
              </button>
            )}
            {!trashMode && (
              <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); onMove(); }}>
                <i className="fa-solid fa-folder-tree" /> Move to…
              </button>
            )}
            {!trashMode && (
              <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); onRename(); }}>
                <i className="fa-solid fa-pen" /> Rename
              </button>
            )}
            <div className="drive-card-menu-sep" />
            {trashMode ? (
              <button type="button" role="menuitem" className="danger" onClick={() => { setMenuOpen(false); onPurge(); }}>
                <i className="fa-solid fa-trash" /> Delete forever
              </button>
            ) : (
              <button type="button" role="menuitem" className="danger" onClick={() => { setMenuOpen(false); onDelete(); }}>
                <i className="fa-solid fa-trash" /> Move to trash
              </button>
            )}
          </div>
        )}
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
        <FileThumb
          token={token}
          id={node.id}
          name={node.name}
          mime={node.mime_type}
          isFolder={node.type === "folder"}
        />
      </button>
    </article>
  );
}
