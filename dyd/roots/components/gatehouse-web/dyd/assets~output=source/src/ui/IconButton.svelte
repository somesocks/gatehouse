<script lang="ts">
  import { Button } from "../lib/vendor/shadcn/button"
  import { cn } from "../lib/utils"
  import type { ComponentProps } from "svelte"

  let {
    children,
    variant = "ghost",
    raised = false,
    ...rest
  }: Omit<
    ComponentProps<typeof Button>,
    "class" | "style" | "href" | "size" | "variant"
  > & {
    variant?: "default" | "ghost"
    raised?: boolean
  } = $props()
</script>

<Button
  variant="ghost"
  size="icon"
  class="group/icon-target size-(--icon-button-target) hover:bg-transparent aria-expanded:bg-transparent dark:hover:bg-transparent [&_svg]:size-(--icon-button-glyph)"
  {...rest}
>
  <span
    class={cn(
      "pointer-events-none inline-flex size-(--icon-button-face) shrink-0 items-center justify-center rounded-lg transition-colors",
      variant === "default"
        ? "bg-primary text-primary-foreground group-hover/icon-target:bg-primary/80"
        : "group-hover/icon-target:bg-muted group-aria-expanded/icon-target:bg-muted dark:group-hover/icon-target:bg-muted/50",
      raised && "shadow-sm",
    )}
  >
    {@render children?.()}
  </span>
</Button>
