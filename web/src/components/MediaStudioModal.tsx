import { useEffect, useState } from "react";
import { CirclePlay, FileAudio, Loader2, Sparkles, Spool, Spool as CompressIcon } from "lucide-react";
import {
  mediaJobStatus,
  startMediaJob,
  type MediaAction,
  type MediaJobStatus,
  type Node,
} from "../api";
import { formatBytes } from "../lib/files";
import { cn } from "@/lib/utils";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import { Progress } from "./ui/progress";

type Props = {
  token: string;
  node: Node;
  busy?: boolean;
  onClose: () => void;
  onDone: () => void;
};

const ACTIONS: {
  id: MediaAction;
  title: string;
  blurb: string;
  icon: typeof CirclePlay;
  tag: string;
}[] = [
  {
    id: "playable",
    title: "Make playable",
    blurb: "Convert to browser-safe H.264/AAC MP4. Your original stays untouched.",
    icon: CirclePlay,
    tag: "Convert",
  },
  {
    id: "extract_audio",
    title: "Extract audio",
    blurb: "Pull the soundtrack out as an .m4a file for reuse in edits.",
    icon: FileAudio,
    tag: "Audio",
  },
  {
    id: "enhance",
    title: "Enhance 1080p",
    blurb: "Upscale, denoise, sharpen and boost color for a cleaner sharing copy.",
    icon: Sparkles,
    tag: "Enhance",
  },
  {
    id: "compress",
    title: "Compress",
    blurb: "Create a smaller H.265 MP4 copy when you want to save space.",
    icon: CompressIcon,
    tag: "Compress",
  },
];

export function MediaStudioModal({ token, node, busy = false, onClose, onDone }: Props) {
  const [action, setAction] = useState<MediaAction>("playable");
  const [job, setJob] = useState<MediaJobStatus | null>(null);
  const [error, setError] = useState("");
  const [starting, setStarting] = useState(false);

  const running = job && (job.status === "queued" || job.status === "running");
  const selectedAction = ACTIONS.find((item) => item.id === action) ?? ACTIONS[0];
  const SelectedIcon = selectedAction.icon;

  useEffect(() => {
    if (!job || job.status === "done" || job.status === "error") return;
    let cancelled = false;
    const timer = window.setInterval(() => {
      mediaJobStatus(token, job.id)
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
  }, [job, token, onDone]);

  async function start() {
    setError("");
    setStarting(true);
    try {
      const { job_id } = await startMediaJob(token, node.id, action);
      setJob({
        id: job_id,
        action,
        status: "queued",
        phase: "queued",
        progress: 0,
        message: "Queued — warming up FFmpeg…",
      });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setStarting(false);
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && !running && onClose()}>
      <DialogContent className="overflow-hidden p-0 sm:max-w-[720px]">
        <DialogHeader className="px-6 pb-0 pt-6">
          <Badge variant="muted" className="w-fit">
            <Spool className="mr-1 h-3 w-3" /> Media Studio
          </Badge>
          <DialogTitle className="text-xl">Create from your media</DialogTitle>
          <DialogDescription>
            Quick FFmpeg magic — results save back to Nimbus. Friendly and fast.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 px-6 py-2 md:grid-cols-[240px_1fr]">
          <div className="grid content-start gap-2 rounded-2xl border bg-muted/40 p-3">
            <div className="grid gap-1 px-1 pb-1">
              <p className="truncate text-sm font-semibold">{node.name}</p>
              <p className="text-xs text-muted-foreground">{formatBytes(node.size)} · source</p>
            </div>
            <div className="flex gap-1.5" aria-hidden>
              <span className="h-8 flex-1 rounded-lg bg-gradient-to-br from-sky-400 to-emerald-300" />
              <span className="h-8 flex-1 rounded-lg bg-gradient-to-br from-purple-400 to-amber-300" />
              <span className="h-8 flex-1 rounded-lg bg-gradient-to-br from-emerald-400 to-sky-300" />
            </div>
            <div role="radiogroup" aria-label="Media action" className="grid gap-1.5 pt-1">
              {ACTIONS.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  role="radio"
                  aria-checked={action === item.id}
                  disabled={!!running || busy}
                  onClick={() => setAction(item.id)}
                  className={cn(
                    "flex items-center gap-2.5 rounded-xl border px-3 py-2.5 text-left text-sm font-medium transition-colors",
                    action === item.id
                      ? "border-ring bg-card shadow-xs ring-2 ring-ring/20"
                      : "border-transparent hover:bg-card",
                  )}
                >
                  <item.icon className="h-4 w-4 shrink-0 text-muted-foreground" />
                  {item.title}
                </button>
              ))}
            </div>
          </div>

          <div className="grid content-center gap-2 rounded-2xl border bg-card p-5">
            <Badge variant="secondary" className="w-fit">
              {selectedAction.tag}
            </Badge>
            <span className="grid h-12 w-12 place-items-center rounded-2xl bg-primary text-primary-foreground">
              <SelectedIcon className="h-5 w-5" />
            </span>
            <h3 className="text-base font-semibold">{selectedAction.title}</h3>
            <p className="text-sm text-muted-foreground">{selectedAction.blurb}</p>
            <div className="mt-1 flex items-center gap-2 rounded-full bg-muted px-3.5 py-2 text-sm">
              <span className="text-muted-foreground">Output</span>
              <strong>{action === "extract_audio" ? "Audio .m4a" : "Video .mp4"}</strong>
            </div>

            {(error || job) && (
              <div className="grid gap-2 pt-1" aria-live="polite">
                {error && (
                  <Alert variant="destructive">
                    <AlertDescription>{error}</AlertDescription>
                  </Alert>
                )}
                {job && (
                  <div className="grid gap-1.5">
                    <div className="flex items-center justify-between text-sm">
                      <span className="text-muted-foreground">{job.message || job.phase}</span>
                      <span className="font-semibold tabular-nums">{Math.round(job.progress)}%</span>
                    </div>
                    <Progress value={Math.min(100, job.progress)} />
                    {job.status === "done" && job.node && (
                      <p className="text-sm text-muted-foreground">
                        Saved as <strong className="text-foreground">{job.node.name}</strong>
                      </p>
                    )}
                    {job.status === "error" && (
                      <Alert variant="destructive">
                        <AlertDescription>{job.message}</AlertDescription>
                      </Alert>
                    )}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        <DialogFooter className="items-center gap-2 bg-muted/40 px-6 py-4 sm:justify-between">
          <span className="text-xs text-muted-foreground">Server FFmpeg job · Telegram-backed output</span>
          <div className="flex gap-2">
            <Button variant="outline" disabled={!!running} onClick={onClose}>
              {job?.status === "done" ? "Close" : "Cancel"}
            </Button>
            {job?.status !== "done" && (
              <Button disabled={!!running || starting || busy} onClick={() => void start()}>
                {(starting || running) && <Loader2 className="h-4 w-4 animate-spin" />}
                {starting || running ? "Working…" : "Run"}
              </Button>
            )}
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
