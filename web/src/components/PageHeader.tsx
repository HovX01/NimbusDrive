import { createContext, useContext, type ReactNode } from "react";
import { cn } from "@/lib/utils";

type Props = {
  title: string;
  description?: string;
  actions?: ReactNode;
  className?: string;
};

/**
 * Optional leading control (e.g. the sidebar collapse toggle) rendered on the
 * same line as the title. DriveShell provides it; pages stay decoupled.
 */
export const HeaderLeadingContext = createContext<ReactNode | null>(null);

/**
 * Consistent minimal page header used across Drive, Trash, Buckets and Settings pages.
 */
export function PageHeader({ title, description, actions, className }: Props) {
  const leading = useContext(HeaderLeadingContext);
  return (
    <header className={cn("flex flex-wrap items-end justify-between gap-3 border-b pb-4", className)}>
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          {leading}
          <h1 className="truncate text-xl font-semibold tracking-tight sm:text-2xl">{title}</h1>
        </div>
        {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}
