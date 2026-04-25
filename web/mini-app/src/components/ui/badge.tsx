import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"

const badgeVariants = cva("inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium", {
  variants: {
    variant: {
      default: "bg-[color:var(--muted)] text-[color:var(--foreground)]",
      success: "bg-[#ddeed6] text-[#2d5f24]",
      warning: "bg-[#f7e6bd] text-[#7a4a0e]",
      destructive: "bg-[#f6d4ce] text-[#8c2818]",
    },
  },
  defaultVariants: {
    variant: "default",
  },
})

export function Badge({ className, variant, ...props }: React.HTMLAttributes<HTMLSpanElement> & VariantProps<typeof badgeVariants>) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />
}
