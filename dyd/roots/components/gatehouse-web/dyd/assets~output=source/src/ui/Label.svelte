<script lang="ts">
  import { Label } from "../lib/vendor/shadcn/label"
  import type { ComponentProps } from "svelte"
  let {
    layout = "inline",
    hidden = false,
    children,
    ...rest
  }: Omit<ComponentProps<typeof Label>, "class" | "style" | "child"> & {
    layout?: "inline" | "field" | "choice"
    hidden?: boolean
  } = $props()
</script>

<Label
  class={hidden
    ? "sr-only"
    : layout === "field"
      ? "grid gap-2"
      : "flex items-center gap-2"}
  {...rest}>{@render children?.()}</Label
>
