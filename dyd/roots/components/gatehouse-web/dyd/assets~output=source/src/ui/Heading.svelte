<script lang="ts">
  import type { Attributes } from "./types"
  let {
    level = 2,
    children,
    ...rest
  }: Attributes & { level?: number } = $props()
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
  class={classes[Math.min(6, Math.max(1, level)) - 1]}
  {...rest}>{@render children?.()}</svelte:element
>
