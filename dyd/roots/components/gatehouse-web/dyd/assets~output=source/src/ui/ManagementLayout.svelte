<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    header,
    children,
    aside,
  }: {
    header: Snippet
    children: Snippet
    aside: Snippet
  } = $props()
</script>

<div class="@container w-full min-w-0 space-y-6">
  {@render header()}
  <div
    class="grid min-w-0 items-start gap-6 @min-[56rem]:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]"
  >
    <div class="min-w-0 space-y-6">{@render children()}</div>
    <aside class="min-w-0 space-y-6">{@render aside()}</aside>
  </div>
</div>
