import { useEffect, useMemo, useState } from "react";
import { ChevronRight, Loader2, Search, Send } from "lucide-react";
import { listBots, listContacts, type TelegramContact } from "../api";
import { ContactAvatar } from "./ContactAvatar";
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
import { Input } from "./ui/input";
import { Skeleton } from "./ui/skeleton";
import { cn } from "@/lib/utils";

type Props = {
  token: string;
  fileName: string;
  busy: boolean;
  error?: string;
  onClose: () => void;
  onSend: (contact: TelegramContact) => void;
};

function contactHandle(c: TelegramContact) {
  if (c.username) return `@${c.username}`;
  return c.display_name;
}

function matchesQuery(c: TelegramContact, query: string) {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return (
    c.display_name.toLowerCase().includes(q) ||
    (c.username?.toLowerCase().includes(q) ?? false) ||
    (c.first_name?.toLowerCase().includes(q) ?? false)
  );
}

export function SendTelegramModal({ token, fileName, busy, error, onClose, onSend }: Props) {
  const [query, setQuery] = useState("");
  const [contacts, setContacts] = useState<TelegramContact[]>([]);
  const [bots, setBots] = useState<TelegramContact[]>([]);
  const [loading, setLoading] = useState(true);
  const [listError, setListError] = useState("");
  const [selected, setSelected] = useState<TelegramContact | null>(null);

  useEffect(() => {
    let cancelled = false;
    const timer = window.setTimeout(() => {
      setLoading(true);
      setListError("");
      Promise.all([listContacts(token, query), listBots(token)])
        .then(([people, botRes]) => {
          if (cancelled) return;
          setContacts(people.items ?? []);
          setBots(
            (botRes.items ?? [])
              .filter((b) => b.allowed)
              .map((b) => ({
                id: b.id,
                username: b.username,
                first_name: b.display_name,
                display_name: b.display_name,
                has_avatar: b.has_avatar,
                is_bot: true,
              })),
          );
        })
        .catch((e) => {
          if (!cancelled) {
            setListError((e as Error).message);
            setContacts([]);
            setBots([]);
          }
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, query ? 250 : 0);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [token, query]);

  const recipients = useMemo(() => {
    const seen = new Set<number>();
    const out: TelegramContact[] = [];
    for (const b of bots) {
      if (!matchesQuery(b, query) || seen.has(b.id)) continue;
      seen.add(b.id);
      out.push(b);
    }
    for (const c of contacts) {
      if (seen.has(c.id)) continue;
      seen.add(c.id);
      out.push(c);
    }
    return out;
  }, [bots, contacts, query]);

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>
            Send <span className="truncate align-middle">{fileName}</span>
          </DialogTitle>
          <DialogDescription>
            Pick a person or an allowed bot — just like sending a file in Telegram.
          </DialogDescription>
        </DialogHeader>

        {busy && (
          <Alert>
            <Loader2 className="h-4 w-4 animate-spin" />
            <AlertDescription>
              Sending to {selected?.display_name ?? "chat"}… this may take a moment for large files.
            </AlertDescription>
          </Alert>
        )}

        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search people or bots"
            autoFocus
            className="pl-9"
          />
        </div>

        {listError && (
          <Alert variant="destructive">
            <AlertDescription>{listError}</AlertDescription>
          </Alert>
        )}
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {loading ? (
          <div className="grid gap-2">
            {[0, 1, 2].map((i) => (
              <div key={i} className="flex items-center gap-3">
                <Skeleton className="h-10 w-10 rounded-full" />
                <div className="grid flex-1 gap-1.5">
                  <Skeleton className="h-3.5 w-1/2" />
                  <Skeleton className="h-3 w-1/3" />
                </div>
              </div>
            ))}
          </div>
        ) : recipients.length === 0 ? (
          <p className="rounded-xl bg-muted/60 px-3.5 py-3 text-sm text-muted-foreground">
            No matches. Allow a bot in Settings, or open it in Telegram (tap Start) first.
          </p>
        ) : (
          <ul className="grid max-h-[320px] gap-1 overflow-auto pr-0.5">
            {recipients.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => setSelected(c)}
                  className={cn(
                    "flex w-full items-center gap-3 rounded-xl px-2.5 py-2 text-left transition-colors hover:bg-muted",
                    selected?.id === c.id && "bg-muted ring-1 ring-ring",
                  )}
                >
                  <ContactAvatar token={token} contact={c} />
                  <span className="grid min-w-0 flex-1">
                    <span className="flex items-center gap-1.5">
                      <span className="truncate text-sm font-medium">{c.display_name}</span>
                      {c.is_bot && <Badge variant="muted">Bot</Badge>}
                    </span>
                    <span className="truncate text-xs text-muted-foreground">{contactHandle(c)}</span>
                  </span>
                  <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                </button>
              </li>
            ))}
          </ul>
        )}

        {selected && !busy && (
          <DialogFooter className="items-center gap-2 border-t pt-4 sm:justify-between">
            <p className="text-sm text-muted-foreground">
              Send to <strong className="text-foreground">{selected.display_name}</strong>?
            </p>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setSelected(null)}>
                Back
              </Button>
              <Button onClick={() => onSend(selected)}>
                <Send /> Send
              </Button>
            </div>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
