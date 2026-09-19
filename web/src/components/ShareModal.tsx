import { useMemo, useState } from "react";
import { Check, Copy, Link2, Mail, MessageCircle, Send, Share as ShareIcon } from "lucide-react";
import type { ShareInfo } from "../api";
import { copyText } from "../lib/clipboard";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import { Separator } from "./ui/separator";

type Props = {
  fileName: string;
  busy: boolean;
  shares: ShareInfo[];
  onClose: () => void;
  onCreate: () => Promise<ShareInfo | null>;
  onRevoke: (shareId: string) => void;
};

function canUseSystemShare() {
  return typeof navigator !== "undefined" && typeof navigator.share === "function";
}

export function ShareModal({ fileName, busy, shares, onClose, onCreate, onRevoke }: Props) {
  const [copied, setCopied] = useState<string | null>(null);
  const [manualCopy, setManualCopy] = useState<string | null>(null);
  const [sharing, setSharing] = useState(false);
  const systemShare = useMemo(() => canUseSystemShare(), []);

  async function copy(url: string) {
    const ok = await copyText(url);
    if (ok) {
      setManualCopy(null);
      setCopied(url);
      window.setTimeout(() => setCopied(null), 2000);
    } else {
      // Clipboard blocked — show the link so it can be selected by hand.
      setManualCopy(url);
    }
  }

  async function ensureUrl(): Promise<string | null> {
    if (shares[0]?.url) return shares[0].url;
    const created = await onCreate();
    return created?.url ?? null;
  }

  async function shareSystem() {
    setSharing(true);
    try {
      const url = await ensureUrl();
      if (!url) return;
      if (systemShare) {
        try {
          await navigator.share({ title: fileName, text: fileName, url });
          return;
        } catch (e) {
          if ((e as Error).name === "AbortError") return;
        }
      }
      await copy(url);
    } finally {
      setSharing(false);
    }
  }

  async function openAppShare(kind: "telegram" | "whatsapp" | "messages") {
    setSharing(true);
    try {
      const url = await ensureUrl();
      if (!url) return;
      const text = `${fileName}\n${url}`;
      let href = "";
      if (kind === "telegram") {
        href = `https://t.me/share/url?url=${encodeURIComponent(url)}&text=${encodeURIComponent(fileName)}`;
      } else if (kind === "whatsapp") {
        href = `https://wa.me/?text=${encodeURIComponent(text)}`;
      } else {
        const sms = `sms:?&body=${encodeURIComponent(text)}`;
        href = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent) ? sms : `mailto:?subject=${encodeURIComponent(fileName)}&body=${encodeURIComponent(text)}`;
      }
      window.open(href, "_blank", "noopener,noreferrer");
    } finally {
      setSharing(false);
    }
  }

  const disabled = busy || sharing;

  const apps = [
    ...(systemShare
      ? [{ id: "system", label: "Share via…", icon: ShareIcon, tint: "bg-primary text-primary-foreground", run: shareSystem }]
      : []),
    { id: "messages", label: "Messages", icon: MessageCircle, tint: "bg-green-500 text-white", run: () => void openAppShare("messages") },
    { id: "whatsapp", label: "WhatsApp", icon: MessageCircle, tint: "bg-[#25D366] text-white", run: () => void openAppShare("whatsapp") },
    { id: "telegram", label: "Telegram", icon: Send, tint: "bg-[#229ED9] text-white", run: () => void openAppShare("telegram") },
    {
      id: "copy",
      label: copied ? "Copied!" : "Copy link",
      icon: copied ? Check : Link2,
      tint: "bg-muted text-foreground",
      run: () => {
        void (async () => {
          setSharing(true);
          try {
            const url = await ensureUrl();
            if (url) await copy(url);
          } finally {
            setSharing(false);
          }
        })();
      },
    },
  ];

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle className="truncate">{fileName}</DialogTitle>
          <DialogDescription>Share a friendly download link — pick your favorite app.</DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-5 gap-2 py-2">
          {apps.map((a) => (
            <button
              key={a.id}
              type="button"
              disabled={disabled}
              onClick={a.run}
              className="group grid justify-items-center gap-1.5 rounded-xl p-2 transition-colors hover:bg-muted disabled:opacity-50"
            >
              <span className={`grid h-11 w-11 place-items-center rounded-2xl shadow-xs ${a.tint}`}>
                <a.icon className="h-5 w-5" />
              </span>
              <span className="text-[11px] font-medium leading-tight">{a.label}</span>
            </button>
          ))}
        </div>

        <p className="text-xs text-muted-foreground">
          Nimbus creates one download link, then opens your share target. Easy!
        </p>

        {manualCopy && (
          <div className="grid gap-1.5 rounded-xl border border-dashed p-3">
            <p className="text-xs font-medium">Copy this link by hand:</p>
            <code className="break-all font-mono text-xs">{manualCopy}</code>
          </div>
        )}

        {shares.length > 0 && (
          <>
            <Separator />
            <ul className="grid gap-2">
              {shares.map((s) => (
                <li
                  key={s.link.id}
                  className="flex items-center gap-2 rounded-xl border bg-muted/40 px-3 py-2"
                >
                  <code className="min-w-0 flex-1 truncate text-xs" title={s.url}>
                    {s.url}
                  </code>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={disabled}
                    onClick={() => void copy(s.url)}
                  >
                    {copied === s.url ? <Check /> : <Copy />}
                    {copied === s.url ? "Copied" : "Copy"}
                  </Button>
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={disabled}
                    onClick={() => onRevoke(s.link.id)}
                  >
                    Revoke
                  </Button>
                </li>
              ))}
            </ul>
          </>
        )}

        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Mail className="h-3.5 w-3.5" />
          Links work for anyone — revoke anytime to take one down.
        </div>
      </DialogContent>
    </Dialog>
  );
}
