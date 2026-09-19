import { useEffect, useState } from "react";
import { Check, ChevronLeft, Container, Copy, Folder, Loader2, Plus, RefreshCw } from "lucide-react";
import {
  createS3Bucket,
  fetchS3BucketObjects,
  fetchS3Buckets,
  type S3Bucket,
  type S3Object,
} from "../api";
import { cn } from "@/lib/utils";
import { copyText } from "../lib/clipboard";
import { PageHeader } from "./PageHeader";
import { EmptyState } from "./EmptyState";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Separator } from "./ui/separator";
import { Skeleton } from "./ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "./ui/table";

type Props = {
  token: string;
};

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(1)} ${units[unit]}`;
}

function shortDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleDateString();
}

export function S3BucketsPage({ token }: Props) {
  const [buckets, setBuckets] = useState<S3Bucket[]>([]);
  const [loading, setLoading] = useState(true);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [openBucket, setOpenBucket] = useState<string | null>(null);
  const [objects, setObjects] = useState<S3Object[] | null>(null);
  const [objectsLoading, setObjectsLoading] = useState(false);
  const [uploadHint, setUploadHint] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    void refresh();
  }, [token]);

  async function refresh() {
    setLoading(true);
    setError("");
    try {
      const r = await fetchS3Buckets(token);
      setBuckets(r.items ?? []);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function handleCreate() {
    const trimmed = name.trim();
    if (!trimmed) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const bucket = await createS3Bucket(token, trimmed);
      setName("");
      setMessage(`Bucket "${bucket.name}" created. Upload to it with rclone or aws-cli.`);
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  function openDetail(bucket: string) {
    setOpenBucket(bucket);
    setObjects(null);
    setUploadHint("");
    setError("");
    setObjectsLoading(true);
    fetchS3BucketObjects(token, bucket)
      .then((r) => {
        setObjects(r.items ?? []);
        setUploadHint(r.upload);
      })
      .catch((e) => setError((e as Error).message))
      .finally(() => setObjectsLoading(false));
  }

  async function copyUploadCommand() {
    if (!uploadHint) return;
    const ok = await copyText(uploadHint);
    if (ok) {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } else {
      setError("Copy blocked by the browser — select the command by hand.");
    }
  }

  return (
    <div className="grid gap-5">
      <PageHeader
        title={openBucket ? openBucket : "S3 Buckets"}
        description={
          openBucket
            ? "Objects stored in this bucket via the S3-compatible gateway."
            : "Each bucket is a top-level folder. Create one, then point your S3 client at it."
        }
        actions={
          openBucket ? (
            <Button variant="outline" onClick={() => setOpenBucket(null)}>
              <ChevronLeft /> All buckets
            </Button>
          ) : (
            <Button variant="outline" disabled={loading} onClick={() => void refresh()}>
              <RefreshCw className={cn(loading && "animate-spin")} /> Refresh
            </Button>
          )
        }
      />

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {message && (
        <Alert>
          <Check className="h-4 w-4 text-green-600" />
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      )}

      {openBucket ? (
        <div className="grid gap-4">
          {uploadHint && (
            <div className="flex items-center gap-2 rounded-xl border bg-card px-3 py-2">
              <code className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{uploadHint}</code>
              <Button variant="outline" size="sm" onClick={() => void copyUploadCommand()}>
                {copied ? <Check /> : <Copy />}
                {copied ? "Copied" : "Copy"}
              </Button>
            </div>
          )}
          {objectsLoading ? (
            <div className="grid gap-2">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-11 w-full" />
              ))}
            </div>
          ) : objects === null || objects.length === 0 ? (
            <EmptyState
              icon={Container}
              title="No objects yet"
              description="Upload files to this bucket using the command above, then refresh."
            />
          ) : (
            <BucketObjectsTable objects={objects} />
          )}
        </div>
      ) : (
        <div className="grid gap-5">
          <div className="grid max-w-xl gap-1.5">
            <Label htmlFor="bucket-name">New bucket</Label>
            <div className="flex gap-2">
              <Input
                id="bucket-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="my-bucket"
                onKeyDown={(e) => {
                  if (e.key === "Enter") void handleCreate();
                }}
              />
              <Button disabled={busy || !name.trim()} onClick={() => void handleCreate()}>
                {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus />}
                Create
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">Lowercase letters, numbers, dots and hyphens · 3–63 characters.</p>
          </div>

          <Separator />

          {loading ? (
            <div className="grid gap-2">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-11 w-full" />
              ))}
            </div>
          ) : buckets.length === 0 ? (
            <EmptyState
              icon={Container}
              title="No buckets yet"
              description="Create your first bucket above to start receiving S3 uploads."
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead className="hidden sm:table-cell">Created</TableHead>
                  <TableHead className="w-[80px] text-right">Objects</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {buckets.map((b) => (
                  <TableRow key={b.name} className="cursor-pointer" onClick={() => openDetail(b.name)}>
                    <TableCell>
                      <span className="flex items-center gap-2.5">
                        <span className="grid h-7 w-7 place-items-center rounded-lg bg-amber-100 text-amber-600">
                          <Folder className="h-3.5 w-3.5" />
                        </span>
                        <span className="font-medium">{b.name}</span>
                      </span>
                    </TableCell>
                    <TableCell className="hidden text-muted-foreground sm:table-cell">
                      <Badge variant="muted">{shortDate(b.created_at)}</Badge>
                    </TableCell>
                    <TableCell className="text-right text-muted-foreground">Open</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      )}
    </div>
  );
}

function BucketObjectsTable({ objects }: { objects: S3Object[] }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Key</TableHead>
          <TableHead className="hidden sm:table-cell">Type</TableHead>
          <TableHead className="w-[100px] text-right">Size</TableHead>
          <TableHead className="hidden md:table-cell">Modified</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {objects.map((o) => (
          <TableRow key={o.key}>
            <TableCell className="max-w-[280px] truncate font-mono text-xs">{o.key}</TableCell>
            <TableCell className="hidden sm:table-cell">
              <Badge variant="muted">{o.mime_type || "file"}</Badge>
            </TableCell>
            <TableCell className="text-right tabular-nums">{formatSize(o.size)}</TableCell>
            <TableCell className="hidden text-muted-foreground md:table-cell">{shortDate(o.modified)}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
