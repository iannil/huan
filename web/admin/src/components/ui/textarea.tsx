import * as React from "react"

import { cn } from "@/lib/utils"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "flex field-sizing-content min-h-16 w-full border border-input bg-transparent px-2 py-2 text-xs transition-none outline-none rounded-md",
        "focus-visible:border-primary",
        "placeholder:text-muted-foreground",
        "disabled:cursor-not-allowed disabled:opacity-50",
        "font-mono",
        className
      )}
      {...props}
    />
  )
}

export { Textarea }
