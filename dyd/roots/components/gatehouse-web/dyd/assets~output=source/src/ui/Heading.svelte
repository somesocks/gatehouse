<script lang="ts">
  import type { Attributes } from "./types"
  let {
    level = 2,
    size = "default",
    children,
    ...rest
  }: Attributes & { level?: number; size?: "default" | "small" } = $props()
  const classes = [
    "text-3xl font-semibold tracking-tight",
    "text-xl font-semibold tracking-tight",
    "text-lg font-semibold",
    "text-base font-semibold",
    "text-sm font-semibold",
    "text-sm font-semibold",
  ]
</script>

<svelte:element
  this={`h${Math.min(6, Math.max(1, level))}`}
  class={size === "small"
    ? "text-base font-semibold leading-tight"
    : classes[Math.min(6, Math.max(1, level)) - 1]}
  {...rest}>{@render children?.()}</svelte:element
>
