<script lang="ts">
  import { setContext } from "svelte"
  import type { Snippet } from "svelte"
  import { sidebarPageContext, type SidebarPageState } from "./context"

  let { className = "", children }: { className?: string; children: Snippet } =
    $props()
  const state = $state<SidebarPageState>({
    drawerOpen: false,
    floatingHeaderHeight: 0,
    floatingFooterHeight: 0,
  })

  setContext(sidebarPageContext, state)
</script>

<div class={`sidebar-page-root brand-workspace-frame ${className}`}>
  {#if state.drawerOpen}<button
      class="sidebar-page-backdrop"
      type="button"
      aria-label="Close navigation menu"
      onclick={() => (state.drawerOpen = false)}
    ></button>{/if}
  {@render children()}
</div>
