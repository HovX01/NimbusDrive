import { useEffect, useState } from "react";
import { Archive, Equal, Monitor, type LucideIcon } from "lucide-react";
import {
  getEditExportStatus,
  startEditExport,
  type EditExportJobStatus,
  type EditProject,
  type ExportPreset,
} from "../../api";
import { Alert, AlertDescription } from "../ui/alert";
import { Button } from "../ui/button";
import { cn } from "@/lib/utils";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";
import { Progress } from "../ui/progress";

type Props = {
  token: string;
  project: EditProject;
  onClose: () => void;
  onDone: () => void;
};

const PRESETS: { id: ExportPreset; label: string; icon: LucideIcon }[] = [
  { id: "match", label: "Match original", icon: Equal },
  { id: "1080p", label: "1080p H.264", icon: Monitor },
  { id: "compressed", label: "Compressed H.265", icon: Archive },
];

export function ExportPanel({ token, project, onClose, onDone }: Props) {
  const [preset, setPreset] = useState<ExportPreset>("match");
  const [job, setJob] = useState<EditExportJobStatus | null>(null);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState("");
  const running = job?.status === "queued" || job?.status === "running";
  const progress = job ? Math.min(100, Math.max(0, job.progress)) : 0;

  useEffect(() => {
    if (!job || job.status === "done" || job.status === "error") return;
    let cancelled = false;
    const timer = window.setInterval(() => {
      getEditExportStatus(token, job.id)
        .then((status) => {
          if (cancelled) return;
          setJob(status);
          if (status.status === "done") onDone();
        })
        .catch((e) => {
          if (!cancelled) setError((e as Error).message);
        });
    }, 900);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [job, onDone, token]);

  async function start() {
    setStarting(true);
    setError("");
    try {
      const { job_id } = await startEditExport(token, project.id, preset);
      setJob({
        id: job_id,
        project_id: project.id,
        status: "queued",
        phase: "queued",
        progress: 0,
        message: "Queued...",
      });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setStarting(false);
    }
  }

  return (
    <Dialog open onOpenChange={(o) => { if (!o && !running) onClose(); }}>
      <DialogContent
        className="sm:max-w-[480px]"
        onInteractOutside={(e) => { if (running) e.preventDefault(); }}
        onEscapeKeyDown={(e) => { if (running) e.preventDefault(); }}
      >
        <DialogHeader>
          <p className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Export</p>
          <DialogTitle>{project.name}</DialogTitle>
          <DialogDescription>Pick a preset, then export your timeline.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-2" role="radiogroup" aria-label="Export preset">
          {PRESETS.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              role="radio"
              aria-checked={preset === id}
              className={cn(
                "flex min-h-[46px] items-center gap-3 rounded-xl border bg-card px-3.5 text-left text-sm font-medium transition-colors",
                preset === id ? "border-primary/40 bg-primary/10 text-foreground" : "hover:bg-muted",
              )}
              disabled={!!running}
              onClick={() => setPreset(id)}
            >
              <Icon className="h-4 w-4" />
              <span>{label}</span>
            </button>
          ))}
        </div>
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        {job && (
          <div className="grid gap-2">
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>{job.message || job.phase}</span>
              <span>{Math.round(job.progress)}%</span>
            </div>
            <Progress value={progress} />
            {job.status === "done" && job.node && <p className="text-xs text-muted-foreground">Saved as <strong className="text-foreground">{job.node.name}</strong></p>}
            {job.status === "error" && (
              <Alert variant="destructive">
                <AlertDescription>{job.message}</AlertDescription>
              </Alert>
            )}
          </div>
        )}
        <DialogFooter className="gap-2">
          <Button type="button" variant="outline" disabled={!!running} onClick={onClose}>
            {job?.status === "done" ? "Close" : "Cancel"}
          </Button>
          {job?.status !== "done" && (
            <Button type="button" disabled={!!running || starting} onClick={() => void start()}>
              {starting || running ? "Exporting..." : "Export"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
