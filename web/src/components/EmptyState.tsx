import type { ComponentType, ReactNode } from "react";
import { cn } from "@/lib/utils";

type Props = {
  icon?: ComponentType<{ className?: string }>;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
};

/**
 * Minimal centered empty state for lists/pages.
 */
export function EmptyState({ icon: Icon, title, description, action, className }: Props) {
  return (
    <div className={cn("grid place-items-center gap-2 rounded-2xl border border-dashed px-6 py-12 text-center", className)}>
      {Icon && (
        <span className="grid h-11 w-11 place-items-center rounded-full bg-muted text-muted-foreground">
          <Icon className="h-5 w-5" />
        </span>
      )}
      <p className="text-sm font-medium">{title}</p>
      {description && <p className="max-w-sm text-sm text-muted-foreground">{description}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
