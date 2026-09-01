import { useEffect, useState } from "react";
import { fetchContactAvatarBlob, type TelegramContact } from "../api";

type Props = {
  token: string;
  contact: TelegramContact;
};

export function ContactAvatar({ token, contact }: Props) {
  const [src, setSrc] = useState<string | null>(null);
  const initial = (contact.display_name.trim()[0] || "?").toUpperCase();

  useEffect(() => {
    if (!contact.has_avatar) {
      setSrc(null);
      return;
    }
    let objectUrl: string | null = null;
    let cancelled = false;
    fetchContactAvatarBlob(token, contact.id)
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
  }, [token, contact.id, contact.has_avatar]);

  return (
    <span className="send-telegram-avatar" aria-hidden>
      {src ? <img src={src} alt="" /> : initial}
    </span>
  );
}
