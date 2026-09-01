import { useEffect, useState } from "react";
import { fetchAvatarBlob, type User } from "../api";

type Props = {
  token: string;
  user: User | null;
};

export function UserBadge({ token, user }: Props) {
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
    <div className="user-badge">
      {src ? (
        <img className="user-avatar" src={src} alt="" />
      ) : (
        <div className="user-avatar fallback" aria-hidden>
          {initial}
        </div>
      )}
      <div className="user-meta truncate">
        <strong className="truncate">{name}</strong>
        <span className="meta truncate">{handle}</span>
      </div>
    </div>
  );
}
