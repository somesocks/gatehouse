<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    element = $bindable(),
    children,
  }: { element?: HTMLElement; children: Snippet } = $props()
</script>

<section bind:this={element} class="min-w-0 flex-1 p-4 md:p-6">
  {@render children()}
</section>
