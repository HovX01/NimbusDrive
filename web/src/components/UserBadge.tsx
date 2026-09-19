import { useEffect, useState } from "react";
import { cn } from "@/lib/utils";
import { fetchAvatarBlob, type User } from "../api";
import { Avatar, AvatarFallback, AvatarImage } from "./ui/avatar";

type Props = {
  token: string;
  user: User | null;
  compact?: boolean;
};

export function UserBadge({ token, user, compact = false }: Props) {
  const [src, setSrc] = useState<string | null>(null);
  const name = user?.first_name || user?.username || "Account";
  const handle = user?.username ? `@${user.username}` : user?.phone || "Telegram";
  const initial = (name.trim()[0] || "?").toUpperCase();

  useEffect(() => {
    if (!user?.has_avatar) {
      setSrc(null);
      return;
    }
    let objectUrl: string | null = null;
    let cancelled = false;
    fetchAvatarBlob(token)
      .then((blob) => {
        if (cancelled) return;
        objectUrl = URL.createObjectURL(blob);
        setSrc(objectUrl);
      })
      .catch(() => {
        if (!cancelled) setSrc(null);
      });
    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [token, user?.has_avatar, user?.telegram_id]);

  return (
    <div
      className={cn(
        "flex min-w-0 items-center gap-2.5 rounded-xl px-1 py-1",
        compact && "lg:min-w-0 lg:gap-0 lg:px-0 lg:justify-center",
      )}
    >
      <Avatar className="h-9 w-9 shrink-0 ring-1 ring-border">
        {src && <AvatarImage src={src} alt="" />}
        <AvatarFallback>{initial}</AvatarFallback>
      </Avatar>
      {!compact && (
        <div className="grid min-w-0 gap-0.5">
          <span className="truncate text-[13px] font-semibold tracking-tight">{name}</span>
          <span className="truncate text-xs text-muted-foreground">{handle}</span>
        </div>
      )}
    </div>
  );
}
