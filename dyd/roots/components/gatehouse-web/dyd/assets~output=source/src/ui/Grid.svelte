<script lang="ts">
  import { cn } from "../lib/utils"
  import type { Attributes } from "./types"
  let {
    columns = 2,
    children,
    ...rest
  }: Attributes & { columns?: 2 | 3 } = $props()
</script>

<div
  class={cn(
    "grid min-w-0 gap-6",
    columns === 2 ? "md:grid-cols-2" : "md:grid-cols-2 xl:grid-cols-3",
  )}
  {...rest}
>
  {@render children?.()}
</div>
