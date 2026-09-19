import { useEffect, useState } from "react";
import { ChevronRight, Folder, FolderOpen, Loader2 } from "lucide-react";
import { listFiles, type Node } from "../api";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import { Skeleton } from "./ui/skeleton";

type Crumb = { id: string; name: string };

type Props = {
  token: string;
  busy: boolean;
  count: number;
  excludeIds: string[];
  onClose: () => void;
  onConfirm: (parentId: string) => void;
};

export function MoveModal({ token, busy, count, excludeIds, onClose, onConfirm }: Props) {
  const [trail, setTrail] = useState<Crumb[]>([{ id: "root", name: "My Drive" }]);
  const [folders, setFolders] = useState<Node[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const parentId = trail[trail.length - 1]?.id ?? "root";

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    listFiles(token, parentId)
      .then((r) => {
        if (cancelled) return;
        setFolders((r.items ?? []).filter((n) => n.type === "folder" && !excludeIds.includes(n.id)));
      })
      .catch((e) => {
        if (!cancelled) setError((e as Error).message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token, parentId, excludeIds]);

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>
            Move {count} item{count === 1 ? "" : "s"}
          </DialogTitle>
          <DialogDescription>Choose a cozy new home for your files.</DialogDescription>
        </DialogHeader>

        <nav aria-label="Folder path" className="flex flex-wrap items-center gap-1 text-sm">
          {trail.map((c, i) => (
            <span key={c.id} className="flex items-center gap-1">
              {i > 0 && <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />}
              <button
                type="button"
                onClick={() => setTrail(trail.slice(0, i + 1))}
                className="rounded-md px-1.5 py-0.5 font-medium text-muted-foreground hover:bg-muted hover:text-foreground aria-[current=true]:text-foreground"
                aria-current={i === trail.length - 1}
              >
                {c.name}
              </button>
            </span>
          ))}
        </nav>

        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {loading ? (
          <div className="grid gap-2">
            {[0, 1, 2].map((i) => (
              <div key={i} className="flex items-center gap-2.5">
                <Skeleton className="h-9 w-9 rounded-xl" />
                <Skeleton className="h-4 flex-1" />
              </div>
            ))}
          </div>
        ) : folders.length === 0 ? (
          <p className="flex items-center gap-2 rounded-xl bg-muted/60 px-3.5 py-3 text-sm text-muted-foreground">
            <FolderOpen className="h-4 w-4" /> No subfolders here — you can move right here.
          </p>
        ) : (
          <ul className="grid max-h-[300px] gap-1 overflow-auto">
            {folders.map((f) => (
              <li key={f.id}>
                <button
                  type="button"
                  onClick={() => setTrail([...trail, { id: f.id, name: f.name }])}
                  className="flex w-full items-center gap-2.5 rounded-xl border px-3 py-2.5 text-left transition-colors hover:bg-muted"
                >
                  <span className="grid h-8 w-8 place-items-center rounded-lg bg-amber-100 text-amber-600">
                    <Folder className="h-4 w-4" />
                  </span>
                  <span className="min-w-0 flex-1 truncate text-sm font-medium">{f.name}</span>
                  <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                </button>
              </li>
            ))}
          </ul>
        )}

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={busy} onClick={() => onConfirm(parentId)}>
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            Move here
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
