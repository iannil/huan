import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center text-xs font-normal whitespace-nowrap transition-none outline-none select-none border rounded-md",
  {
    variants: {
      variant: {
        default:
          "border-primary bg-primary text-primary-foreground hover:bg-foreground hover:text-background",
        outline:
          "border-border bg-transparent text-muted-foreground hover:text-foreground hover:border-foreground",
        ghost:
          "border-transparent text-muted-foreground hover:text-foreground",
        destructive:
          "border-destructive/50 text-destructive hover:bg-destructive hover:text-destructive-foreground",
        link:
          "border-transparent text-primary underline-offset-2 hover:underline",
      },
      size: {
        default: "h-7 px-2",
        sm: "h-6 px-1.5 text-[11px]",
        lg: "h-8 px-3",
        icon: "h-7 w-7",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
  return (
    <ButtonPrimitive
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
