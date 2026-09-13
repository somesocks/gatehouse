<script lang="ts">
  import type { Snippet } from "svelte"
  import { useSidebarPage } from "./context"

  let { children }: { children: Snippet } = $props()
  const state = useSidebarPage()
</script>

<aside
  class:sidebar-page-sidebar--open={state.drawerOpen}
  class="sidebar-page-sidebar"
>
  {@render children()}
</aside>
