<script lang="ts">
  import type { Snippet } from "svelte"
  import { useSidebarPage } from "./context"

  let { children }: { children: Snippet } = $props()
  const pageState = useSidebarPage()
</script>

<span
  class="sidebar-page-toggle"
  role="presentation"
  onclick={() => (pageState.drawerOpen = true)}
  onkeydown={(event) => {
    if (event.key === "Enter" || event.key === " ") pageState.drawerOpen = true
  }}>{@render children()}</span
>
