<script lang="ts">
  import type { Attributes } from "./types"
  let {
    element = $bindable(),
    children,
    ...rest
  }: Attributes & { element?: HTMLElement } = $props()
</script>

<div
  bind:this={element}
  data-slot="feed"
  class="flex min-h-0 w-full min-w-0 flex-1 flex-col gap-6 overflow-x-hidden overflow-y-auto overscroll-contain bg-background p-6"
  {...rest}
>
  {@render children?.()}
</div>
