import * as React from "react"
import { cn } from "@/lib/utils"

const Input = React.forwardRef<HTMLInputElement, React.ComponentProps<"input">>(({ className, ...props }, ref) => {
  return (
    <input
      className={cn(
        "flex h-11 w-full rounded-xl border border-[color:var(--border)] bg-white/70 px-3 py-2 text-sm shadow-sm outline-none transition-colors placeholder:text-[color:var(--muted-foreground)] focus:border-[color:var(--primary)]",
        className,
      )}
      ref={ref}
      {...props}
    />
  )
})
Input.displayName = "Input"

export { Input }
