import { useState } from "react";
import {
  Download,
  ExternalLink,
  Film,
  FolderInput,
  Link2,
  MoreVertical,
  PenLine,
  Scissors,
  Send,
  Trash2,
} from "lucide-react";
import type { Node as DriveNode } from "../api";
import { isNodeDrag, readNodeDragData, setNodeDragData } from "../lib/drag";
import { previewKind } from "../lib/files";
import { cn } from "@/lib/utils";
import { FileThumb, KindIcon } from "./FileThumb";
import { Button } from "./ui/button";
import { Checkbox } from "./ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./ui/dropdown-menu";

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
  const [dropOver, setDropOver] = useState(false);
  const kind = previewKind(node.name, node.mime_type, node.type === "folder");

  const canDrag = movable && !trashMode;
  const canDrop = dropTarget && node.type === "folder" && !trashMode;

  return (
    <article
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
      className={cn(
        "group relative flex flex-col overflow-hidden rounded-2xl border bg-card transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md",
        selected ? "border-sky-600 ring-2 ring-sky-600/25" : "border-border/80",
        dropOver && "border-sky-600 bg-sky-50 ring-2 ring-sky-600/30",
      )}
    >
      <div className="flex min-h-[52px] items-center gap-1.5 bg-muted/40 py-1.5 pl-2 pr-1.5">
        <span
          className={cn(
            "grid h-7 w-7 shrink-0 place-items-center transition-opacity",
            selectionMode || selected ? "opacity-100" : "opacity-0 group-hover:opacity-100",
          )}
          onClick={(e) => e.stopPropagation()}
        >
          <Checkbox
            checked={selected}
            onCheckedChange={onToggleSelect}
            aria-label={`Select ${node.name}`}
          />
        </span>
        <button
          type="button"
          onClick={onOpen}
          title={node.name}
          className="flex min-w-0 flex-1 items-center gap-2 rounded-lg px-1.5 py-1 text-left hover:bg-muted/60"
        >
          <KindIcon kind={kind} />
          <span className="grid min-w-0 flex-1">
            <span className="truncate text-[13px] font-semibold tracking-tight">{node.name}</span>
            {subtitle && <span className="truncate text-xs text-muted-foreground">{subtitle}</span>}
          </span>
        </button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Actions for ${node.name}`}
              onClick={(e) => e.stopPropagation()}
            >
              <MoreVertical className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-52">
            {!trashMode && (
              <DropdownMenuItem onSelect={onOpen}>
                <ExternalLink /> Open
              </DropdownMenuItem>
            )}
            {!trashMode && node.type === "file" && (
              <DropdownMenuItem onSelect={onDownload}>
                <Download /> Download
              </DropdownMenuItem>
            )}
            {!trashMode && node.type === "file" && (
              <DropdownMenuItem onSelect={onShare}>
                <Link2 /> Share link
              </DropdownMenuItem>
            )}
            {!trashMode && node.type === "file" && (
              <DropdownMenuItem onSelect={onSendTelegram}>
                <Send /> Send to Telegram
              </DropdownMenuItem>
            )}
            {!trashMode && node.type === "file" && onMediaStudio && (
              <DropdownMenuItem onSelect={onMediaStudio}>
                <Film /> Media Studio
              </DropdownMenuItem>
            )}
            {!trashMode && (kind === "video" || kind === "image") && onEditVideo && (
              <DropdownMenuItem onSelect={onEditVideo}>
                <Scissors /> Edit media
              </DropdownMenuItem>
            )}
            {!trashMode && (
              <DropdownMenuItem onSelect={onMove}>
                <FolderInput /> Move to…
              </DropdownMenuItem>
            )}
            {!trashMode && (
              <DropdownMenuItem onSelect={onRename}>
                <PenLine /> Rename
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            {trashMode ? (
              <DropdownMenuItem
                onSelect={onPurge}
                className="text-destructive focus:text-destructive [&_svg]:text-destructive"
              >
                <Trash2 /> Delete forever
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem
                onSelect={onDelete}
                className="text-destructive focus:text-destructive [&_svg]:text-destructive"
              >
                <Trash2 /> Move to trash
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <button
        type="button"
        onClick={(e) => {
          if (e.metaKey || e.ctrlKey || selectionMode) {
            onToggleSelect();
            return;
          }
          onOpen();
        }}
        aria-label={selectionMode ? `Select ${node.name}` : `Open ${node.name}`}
        className="relative block w-full cursor-pointer overflow-hidden border-t bg-card text-left"
      >
        {!trashMode && (kind === "video" || kind === "image") && onEditVideo && (
          <span
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
            className="absolute bottom-2.5 left-2.5 z-10 inline-flex items-center gap-1.5 rounded-lg bg-zinc-900/85 px-2.5 py-1.5 text-xs font-semibold text-white backdrop-blur transition-colors hover:bg-sky-600"
          >
            <Scissors className="h-3 w-3" /> Edit
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
    </article>
  );
}
