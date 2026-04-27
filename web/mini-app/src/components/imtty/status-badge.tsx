import { cn } from "@/lib/utils"
import { STATUS_LABEL, type SessionStatus } from "@/lib/imtty-types"

const STATUS_STYLES: Record<SessionStatus, { dot: string; text: string; bg: string }> = {
  running: {
    dot: "bg-blue-500",
    text: "text-blue-700",
    bg: "bg-blue-50",
  },
  starting: {
    dot: "bg-blue-400 animate-pulse",
    text: "text-blue-700",
    bg: "bg-blue-50",
  },
  disconnected: {
    dot: "bg-amber-500",
    text: "text-amber-700",
    bg: "bg-amber-50",
  },
  exited: {
    dot: "bg-muted-foreground",
    text: "text-muted-foreground",
    bg: "bg-muted",
  },
  idle: {
    dot: "bg-muted-foreground/60",
    text: "text-muted-foreground",
    bg: "bg-muted",
  },
}

export function StatusBadge({
  status,
  size = "sm",
  className,
}: {
  status: SessionStatus
  size?: "sm" | "xs"
  className?: string
}) {
  const s = STATUS_STYLES[status]
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full font-medium",
        size === "sm" ? "px-2 py-0.5 text-xs" : "px-1.5 py-0.5 text-[10px]",
        s.bg,
        s.text,
        className,
      )}
    >
      <span className={cn("h-1.5 w-1.5 rounded-full", s.dot)} aria-hidden="true" />
      {STATUS_LABEL[status]}
    </span>
  )
}
