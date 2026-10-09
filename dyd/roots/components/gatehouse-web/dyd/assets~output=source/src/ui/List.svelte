<script lang="ts">
  import type { Attributes } from "./types"
  let {
    scroll = false,
    children,
    ...rest
  }: Attributes & { scroll?: boolean } = $props()
</script>

<div
  class={scroll
    ? "flex max-h-96 flex-col gap-2 overflow-auto"
    : "flex min-w-0 flex-col gap-2"}
  {...rest}
>
  {@render children?.()}
</div>
