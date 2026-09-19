import { useEffect, useState } from "react";
import { fetchContactAvatarBlob, type TelegramContact } from "../api";
import { Avatar, AvatarFallback, AvatarImage } from "./ui/avatar";
import { cn } from "@/lib/utils";

type Props = {
  token: string;
  contact: TelegramContact;
  className?: string;
};

export function ContactAvatar({ token, contact, className }: Props) {
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
    <Avatar className={cn("h-10 w-10", className)}>
      {src && <AvatarImage src={src} alt="" />}
      <AvatarFallback>{initial}</AvatarFallback>
    </Avatar>
  );
}
