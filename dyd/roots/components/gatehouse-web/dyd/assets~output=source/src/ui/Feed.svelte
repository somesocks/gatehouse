<script lang="ts">
  import type { Snippet } from "svelte"
  import type { Attributes } from "./types"
  let {
    element = $bindable(),
    children,
    header,
    actions,
    ...rest
  }: Attributes & {
    element?: HTMLElement
    header?: Snippet
    actions?: Snippet
  } = $props()
</script>

<div class="relative flex min-h-0 w-full min-w-0 flex-1 flex-col">
  {#if header}
    <div
      data-slot="feed-header"
      class="flex max-h-[45%] min-h-0 shrink-0 flex-col gap-4 overflow-y-auto border-b bg-background p-4 sm:p-6"
    >
      {@render header()}
    </div>
  {/if}
  <div
    bind:this={element}
    data-slot="feed"
    class="flex min-h-0 w-full min-w-0 flex-1 flex-col gap-4 overflow-x-hidden overflow-y-auto overscroll-contain bg-background p-4 sm:p-6"
    {...rest}
  >
    {@render children?.()}
  </div>
  {@render actions?.()}
</div>
