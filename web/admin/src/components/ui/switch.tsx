import { Switch as SwitchNamespace } from "@base-ui/react/switch"
import { cn } from "@/lib/utils"

function Switch({
  className,
  ...props
}: SwitchNamespace.Root.Props) {
  return (
    <SwitchNamespace.Root
      data-slot="switch"
      className={cn(
        "peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border border-input bg-muted px-0.5 transition-colors",
        "data-[checked]:bg-primary data-[checked]:border-primary",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className
      )}
      {...props}
    >
      <SwitchNamespace.Thumb
        data-slot="switch-thumb"
        className={cn(
          "block h-3.5 w-3.5 rounded-full bg-background shadow-xs transition-transform",
          "data-[checked]:translate-x-[calc(100%+1px)]",
        )}
      />
    </SwitchNamespace.Root>
  )
}

export { Switch }
