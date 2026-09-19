import { LogOut, UserRound } from "lucide-react";
import type { User } from "../api";
import { cn } from "@/lib/utils";
import { UserBadge } from "./UserBadge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./ui/dropdown-menu";

type Props = {
  token: string;
  user: User | null;
  onLogout: () => void;
  collapsed?: boolean;
};

/**
 * Clickable user chip that opens an account menu. Renders icon-only when the
 * sidebar is collapsed on large screens; the mobile drawer stays full-width.
 */
export function UserMenu({ token, user, onLogout, collapsed = false }: Props) {
  const name = user?.first_name || user?.username || "Account";
  const handle = user?.username ? `@${user.username}` : user?.phone || "Telegram";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="Account menu"
          className={cn(
            "w-full rounded-xl text-left transition-colors hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            collapsed && "lg:grid lg:place-items-center",
          )}
        >
          <UserBadge token={token} user={user} compact={collapsed} />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="top" className="w-56">
        <DropdownMenuLabel className="flex items-center gap-2 font-normal">
          <UserRound className="h-4 w-4 text-muted-foreground" />
          <span className="grid min-w-0">
            <span className="truncate text-sm font-semibold">{name}</span>
            <span className="truncate text-xs text-muted-foreground">{handle}</span>
          </span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          className="text-destructive focus:text-destructive"
          onSelect={onLogout}
        >
          <LogOut className="h-4 w-4" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
