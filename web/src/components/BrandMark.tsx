import { cn } from "@/lib/utils";

type Props = {
  large?: boolean;
  showWordmark?: boolean;
  sub?: string;
};

export function BrandMark({ large = false, showWordmark = true, sub }: Props) {
  return (
    <div className="flex items-center gap-2.5">
      <img
        className={cn(
          "rounded-xl object-cover ring-1 ring-border shadow-xs",
          large ? "h-10 w-10" : "h-7 w-7 rounded-lg",
        )}
        src="/logo.png"
        alt="Nimbus"
        width={large ? 40 : 28}
        height={large ? 40 : 28}
      />
      {showWordmark && (
        <span className="flex flex-col leading-none">
          <span
            className={cn(
              "font-semibold tracking-tight text-foreground",
              large ? "text-[19px]" : "text-[15px]",
            )}
          >
            Nimbus
          </span>
          {sub ? (
            <span className="text-xs text-muted-foreground">{sub}</span>
          ) : large ? (
            <span className="text-xs text-muted-foreground">Your cozy cloud drive</span>
          ) : null}
        </span>
      )}
    </div>
  );
}
