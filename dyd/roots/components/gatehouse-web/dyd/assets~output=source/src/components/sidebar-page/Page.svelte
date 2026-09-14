<script lang="ts">
  import type { Snippet } from "svelte"

  let {
    element = $bindable<HTMLElement | undefined>(undefined),
    children,
  }: { element?: HTMLElement; children: Snippet } = $props()
</script>

<main bind:this={element} class="sidebar-page-page">{@render children()}</main>
