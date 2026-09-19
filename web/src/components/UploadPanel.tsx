import { useEffect } from "react";
import {
  Archive,
  Check,
  CircleAlert,
  Clock,
  CloudUpload,
  File as FileIcon,
  FileText,
  FileImage,
  FileVideo,
  Folder,
  Loader2,
  Music,
  X,
} from "lucide-react";
import { formatBytes, previewKind, type PreviewKind } from "../lib/files";
import { cn } from "@/lib/utils";
import { Button } from "./ui/button";
import { Progress } from "./ui/progress";

export type UploadJob = {
  id: string;
  name: string;
  size: number;
  progress: number;
  status: "queued" | "uploading" | "processing" | "done" | "error";
  error?: string;
};

const KIND_ICON: Record<PreviewKind, typeof FileIcon> = {
  folder: Folder,
  image: FileImage,
  video: FileVideo,
  audio: Music,
  pdf: FileText,
  text: FileText,
  archive: Archive,
  file: FileIcon,
};

type Props = {
  jobs: UploadJob[];
  onClose: () => void;
  onDismiss: (id: string) => void;
};

export function UploadPanel({ jobs, onClose, onDismiss }: Props) {
  const active = jobs.filter(
    (j) => j.status === "queued" || j.status === "uploading" || j.status === "processing",
  ).length;
  const done = jobs.filter((j) => j.status === "done").length;
  const failed = jobs.filter((j) => j.status === "error").length;
  const allSettled = active === 0;

  useEffect(() => {
    if (!jobs.length) return;
    if (!allSettled || failed > 0) return;
    const timer = window.setTimeout(onClose, 2000);
    return () => window.clearTimeout(timer);
  }, [allSettled, failed, onClose, jobs.length]);

  if (!jobs.length) return null;

  return (
    <div
      role="status"
      aria-live="polite"
      className="fixed bottom-4 right-4 z-[1050] w-[min(360px,calc(100vw-2rem))] overflow-hidden rounded-2xl border bg-card shadow-xl"
    >
      <div className="flex items-center gap-3 border-b bg-muted/40 px-4 py-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary text-primary-foreground">
          {allSettled ? (
            failed ? (
              <CircleAlert className="h-4 w-4" />
            ) : (
              <Check className="h-4 w-4" />
            )
          ) : (
            <CloudUpload className="h-4 w-4" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">
            {allSettled
              ? failed
                ? `${failed} upload${failed === 1 ? "" : "s"} failed`
                : "Upload complete — all cozy!"
              : `Uploading ${Math.min(done + failed + 1, jobs.length)} of ${jobs.length}`}
          </p>
          <p className="text-xs text-muted-foreground">
            {allSettled
              ? `${done} done${failed ? ` · ${failed} error${failed === 1 ? "" : "s"}` : ""}`
              : `${active} in progress`}
          </p>
        </div>
        <Button variant="ghost" size="icon-sm" aria-label="Close upload panel" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <ul className="max-h-[320px] divide-y overflow-auto">
        {jobs.map((job) => {
          const kind = previewKind(job.name, "", false);
          const Icon = KIND_ICON[kind];
          return (
            <li key={job.id} className="flex items-start gap-3 px-4 py-3">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-muted text-muted-foreground">
                <Icon className="h-4 w-4" />
              </div>
              <div className="grid min-w-0 flex-1 gap-1">
                <p className="truncate text-[13px] font-medium" title={job.name}>
                  {job.name}
                </p>
                <p className="text-xs text-muted-foreground">
                  {job.status === "error"
                    ? job.error || "Failed"
                    : job.status === "done"
                      ? `Saved · ${formatBytes(job.size)}`
                      : job.status === "processing"
                        ? `Storing in Nimbus · ${formatBytes(job.size)}`
                        : job.status === "uploading"
                          ? `${job.progress}% · ${formatBytes(job.size)}`
                          : `Queued · ${formatBytes(job.size)}`}
                </p>
                {(job.status === "uploading" ||
                  job.status === "queued" ||
                  job.status === "processing") && (
                  <Progress
                    value={job.status === "uploading" ? job.progress : job.status === "processing" ? 100 : 8}
                    className={cn(job.status !== "uploading" && "opacity-70")}
                  />
                )}
              </div>
              <div className="flex shrink-0 items-center gap-1 text-muted-foreground">
                {(job.status === "uploading" || job.status === "processing") && (
                  <Loader2 className="h-4 w-4 animate-spin" />
                )}
                {job.status === "queued" && <Clock className="h-4 w-4" />}
                {job.status === "done" && <Check className="h-4 w-4 text-green-600" />}
                {job.status === "error" && <CircleAlert className="h-4 w-4 text-destructive" />}
                {allSettled && (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Dismiss ${job.name}`}
                    onClick={() => onDismiss(job.id)}
                  >
                    <X className="h-3.5 w-3.5" />
                  </Button>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
