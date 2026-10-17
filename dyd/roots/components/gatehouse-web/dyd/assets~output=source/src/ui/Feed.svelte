<script lang="ts">
  import type { Snippet } from "svelte"
  import type { Attributes } from "./types"
  let {
    element = $bindable(),
    children,
    actions,
    ...rest
  }: Attributes & { element?: HTMLElement; actions?: Snippet } = $props()
</script>

<div class="relative flex min-h-0 w-full min-w-0 flex-1">
  <div
    bind:this={element}
    data-slot="feed"
    class="flex min-h-0 w-full min-w-0 flex-1 flex-col gap-6 overflow-x-hidden overflow-y-auto overscroll-contain bg-background p-6"
    {...rest}
  >
    {@render children?.()}
  </div>
  {@render actions?.()}
</div>
