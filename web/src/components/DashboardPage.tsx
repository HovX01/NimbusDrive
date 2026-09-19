import { useEffect, useMemo, useState } from "react";
import { ChartColumn, FileDigit, FolderOpen, Gauge, RadioTower } from "lucide-react";
import { fetchStorageStats, type StorageStats } from "../api";
import { formatBytes } from "../lib/files";
import { cn } from "@/lib/utils";
import { PageHeader } from "./PageHeader";
import { Badge } from "./ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Progress } from "./ui/progress";
import { Skeleton } from "./ui/skeleton";

type Props = {
  token: string;
};

type RangeKey = "7d" | "30d" | "90d";

const RANGES: { key: RangeKey; label: string; days: number }[] = [
  { key: "7d", label: "7 days", days: 7 },
  { key: "30d", label: "30 days", days: 30 },
  { key: "90d", label: "90 days", days: 90 },
];

const KIND_COLORS: Record<string, string> = {
  video: "bg-chart-1",
  image: "bg-chart-2",
  audio: "bg-chart-3",
  other: "bg-chart-4",
};

function kindLabel(kind: string): string {
  switch (kind) {
    case "video":
      return "Video";
    case "image":
      return "Images";
    case "audio":
      return "Audio";
    default:
      return "Other";
  }
}

function StatCard({
  icon: Icon,
  label,
  value,
  hint,
}: {
  icon: typeof Gauge;
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold tabular-nums">{value}</div>
        {hint && <p className="mt-1 text-xs text-muted-foreground">{hint}</p>}
      </CardContent>
    </Card>
  );
}

export function DashboardPage({ token }: Props) {
  const [stats, setStats] = useState<StorageStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [range, setRange] = useState<RangeKey>("30d");

  const days = RANGES.find((r) => r.key === range)?.days ?? 30;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetchStorageStats(token, days)
      .then((s) => {
        if (!cancelled) setStats(s);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token, days]);

  const kindTotal = useMemo(
    () => (stats?.by_kind ?? []).reduce((sum, k) => sum + k.bytes, 0),
    [stats],
  );
  const channelTotal = useMemo(
    () => (stats?.by_channel ?? []).reduce((sum, c) => sum + c.bytes, 0),
    [stats],
  );
  const dailyMax = useMemo(
    () => Math.max(1, ...(stats?.daily ?? []).map((d) => d.bytes)),
    [stats],
  );

  if (loading && !stats) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-52" />
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full" />
          ))}
        </div>
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (error && !stats) {
    return (
      <div className="space-y-4">
        <PageHeader title="Dashboard" description="Storage usage and activity at a glance." />
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            Could not load storage stats: {error}
          </CardContent>
        </Card>
      </div>
    );
  }

  const s = stats!;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Dashboard"
        description="Storage usage and activity at a glance."
        actions={
          <div className="flex items-center gap-0.5 rounded-lg border p-0.5">
            {RANGES.map((r) => (
              <button
                key={r.key}
                type="button"
                onClick={() => setRange(r.key)}
                aria-pressed={range === r.key}
                className={cn(
                  "rounded-md px-2.5 py-1 text-xs font-medium transition-colors",
                  range === r.key
                    ? "bg-muted text-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {r.label}
              </button>
            ))}
          </div>
        }
      />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          icon={Gauge}
          label="Stored"
          value={formatBytes(s.total_bytes)}
          hint={`${s.total_files.toLocaleString()} files`}
        />
        <StatCard
          icon={FileDigit}
          label="Files"
          value={s.total_files.toLocaleString()}
          hint={`${s.total_folders.toLocaleString()} folders`}
        />
        <StatCard
          icon={RadioTower}
          label="Telegram channels"
          value={(s.by_channel?.length ?? 0).toLocaleString()}
          hint="channels holding file bytes"
        />
        <StatCard
          icon={FolderOpen}
          label="In trash"
          value={formatBytes(s.trash_bytes)}
          hint={`${s.trash_files.toLocaleString()} files`}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        {/* Activity — inline CSS bars, no chart dependency */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <ChartColumn className="h-4 w-4 text-muted-foreground" />
              Bytes uploaded
            </CardTitle>
            <CardDescription>
              Last {days} days · by upload date
            </CardDescription>
          </CardHeader>
          <CardContent>
            {!s.daily || s.daily.length === 0 ? (
              <p className="py-10 text-center text-sm text-muted-foreground">
                No uploads in this window.
              </p>
            ) : (
              <div className="flex h-40 items-end gap-1">
                {s.daily.map((d) => (
                  <div
                    key={d.day}
                    className="group relative flex h-full flex-1 flex-col justify-end"
                    title={`${d.day}: ${formatBytes(d.bytes)} · ${d.count} files`}
                  >
                    <div
                      className="w-full rounded-t bg-primary/80 transition-colors group-hover:bg-primary"
                      style={{ height: `${Math.max(2, (d.bytes / dailyMax) * 100)}%` }}
                    />
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Breakdown by kind */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">By file type</CardTitle>
            <CardDescription>Ready files only</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {!s.by_kind || s.by_kind.length === 0 ? (
              <p className="py-10 text-center text-sm text-muted-foreground">Nothing stored yet.</p>
            ) : (
              s.by_kind.map((k) => {
                const pct = kindTotal > 0 ? (k.bytes / kindTotal) * 100 : 0;
                return (
                  <div key={k.kind} className="space-y-1.5">
                    <div className="flex items-center justify-between text-sm">
                      <span className="flex items-center gap-2">
                        <span
                          className={cn(
                            "h-2.5 w-2.5 rounded-full",
                            KIND_COLORS[k.kind] ?? "bg-chart-4",
                          )}
                        />
                        {kindLabel(k.kind)}
                        <span className="text-muted-foreground">
                          ({k.count.toLocaleString()})
                        </span>
                      </span>
                      <span className="tabular-nums text-muted-foreground">
                        {formatBytes(k.bytes)}
                      </span>
                    </div>
                    <Progress value={pct} />
                  </div>
                );
              })
            )}
          </CardContent>
        </Card>
      </div>

      {/* Per-channel breakdown */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">By Telegram channel</CardTitle>
          <CardDescription>
            Bytes are stored as documents across channels. The S3 gateway keeps
            its own separate buckets.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!s.by_channel || s.by_channel.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              No channel-hosted files.
            </p>
          ) : (
            <div className="divide-y">
              {s.by_channel.map((c) => {
                const pct = channelTotal > 0 ? (c.bytes / channelTotal) * 100 : 0;
                return (
                  <div
                    key={c.channel_id}
                    className="flex items-center gap-4 py-2.5 text-sm first:pt-0 last:pb-0"
                  >
                    <Badge variant="secondary" className="shrink-0 tabular-nums">
                      #{c.channel_id}
                    </Badge>
                    <div className="min-w-0 flex-1">
                      <Progress value={pct} />
                    </div>
                    <span className="w-24 shrink-0 text-right tabular-nums text-muted-foreground">
                      {formatBytes(c.bytes)}
                    </span>
                    <span className="hidden w-20 shrink-0 text-right tabular-nums text-muted-foreground sm:block">
                      {c.count.toLocaleString()} files
                    </span>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
