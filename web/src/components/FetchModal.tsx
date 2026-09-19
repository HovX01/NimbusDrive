import { useEffect, useState } from "react";
import { CloudDownload, FileUp } from "lucide-react";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import { Label } from "./ui/label";
import { Progress } from "./ui/progress";
import { Tabs, TabsList, TabsTrigger } from "./ui/tabs";
import { Textarea } from "./ui/textarea";
import { cn } from "@/lib/utils";

type Kind = "media" | "file";

export function parseFetchURLs(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of text.split(/[\s]+/)) {
    const u = part.trim();
    if (!/^https?:\/\//i.test(u)) continue;
    const key = u.replace(/\/+$/, "");
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(u);
  }
  return out;
}

type Props = {
  busy?: boolean;
  progress?: number;
  message?: string;
  initialUrl?: string;
  onClose: () => void;
  onFetch: (input: { urls: string[]; mode: "video" | "audio"; maxHeight: number | null }) => void;
  onImport: (urls: string[]) => void;
};

export function FetchModal({
  busy,
  progress = 0,
  message,
  initialUrl = "",
  onClose,
  onFetch,
  onImport,
}: Props) {
  const [kind, setKind] = useState<Kind>("media");
  const [url, setUrl] = useState(initialUrl);
  const [mode, setMode] = useState<"video" | "audio">("video");
  const [maxHeight, setMaxHeight] = useState("max");

  useEffect(() => {
    if (initialUrl.trim()) setUrl(initialUrl.trim());
  }, [initialUrl]);

  const urls = parseFetchURLs(url);

  function submit() {
    if (!urls.length || busy) return;
    if (kind === "file") {
      onImport(urls);
      return;
    }
    onFetch({
      urls,
      mode,
      maxHeight: mode === "audio" ? null : maxHeight === "max" ? 0 : Number(maxHeight),
    });
  }

  const pct = Math.max(0, Math.min(100, Math.round(progress)));

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>Fetch from link</DialogTitle>
          <DialogDescription>
            {kind === "media"
              ? "Paste links — highest quality by default, duplicates are skipped. Cozy!"
              : "Direct file URLs (images, PDFs, etc.)."}
          </DialogDescription>
        </DialogHeader>

        <Tabs value={kind} onValueChange={(v) => setKind(v as Kind)}>
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="media" disabled={busy}>
              <CloudDownload className="mr-1.5 h-4 w-4" /> Photos & video
            </TabsTrigger>
            <TabsTrigger value="file" disabled={busy}>
              <FileUp className="mr-1.5 h-4 w-4" /> Direct file
            </TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="grid gap-1.5">
          <Label htmlFor="fetch-urls">URL{urls.length > 1 ? `s (${urls.length})` : ""}</Label>
          <Textarea
            id="fetch-urls"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder={kind === "file" ? "https://example.com/photo.jpg" : "https://… (one per line)"}
            rows={4}
            autoFocus
            disabled={busy}
          />
        </div>

        {kind === "media" && (
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label>Save as</Label>
              <div className="flex gap-1.5 rounded-xl bg-muted p-1">
                {(["video", "audio"] as const).map((m) => (
                  <button
                    key={m}
                    type="button"
                    disabled={busy}
                    onClick={() => setMode(m)}
                    className={cn(
                      "flex-1 rounded-lg px-2 py-1.5 text-[13px] font-medium transition-colors",
                      mode === m ? "bg-card shadow-xs" : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {m === "video" ? "Video" : "Audio"}
                  </button>
                ))}
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="fetch-quality">Quality</Label>
              <select
                id="fetch-quality"
                data-slot="select"
                value={maxHeight}
                onChange={(e) => setMaxHeight(e.target.value)}
                disabled={busy || mode === "audio"}
                className="h-10 w-full rounded-xl border border-input bg-card px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
              >
                <option value="max">Original (max)</option>
                <option value="2160">4K</option>
                <option value="1440">1440p</option>
                <option value="1080">1080p</option>
                <option value="720">720p</option>
                <option value="480">480p</option>
                <option value="360">360p (fastest)</option>
              </select>
            </div>
          </div>
        )}

        {busy && (
          <div className="grid gap-2" aria-live="polite">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">{message || "Working…"}</span>
              <strong className="tabular-nums">{pct}%</strong>
            </div>
            <Progress value={pct} />
          </div>
        )}

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button disabled={busy || !urls.length} onClick={submit}>
            {busy
              ? `${pct}%`
              : kind === "file"
                ? urls.length > 1
                  ? `Import ${urls.length}`
                  : "Import to Drive"
                : urls.length > 1
                  ? `Fetch ${urls.length}`
                  : "Fetch to Drive"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
