<script lang="ts">
  import type { Snippet } from "svelte"
  import { useSidebarPage } from "./context"

  let {
    element = $bindable<HTMLElement | undefined>(undefined),
    children,
  }: { element?: HTMLElement; children: Snippet } = $props()
  const state = useSidebarPage()
</script>

<section
  bind:this={element}
  class="sidebar-page-body"
  style={`--sidebar-page-floating-header-height: ${state.floatingHeaderHeight}px; --sidebar-page-floating-footer-height: ${state.floatingFooterHeight}px`}
>
  {@render children()}
</section>
