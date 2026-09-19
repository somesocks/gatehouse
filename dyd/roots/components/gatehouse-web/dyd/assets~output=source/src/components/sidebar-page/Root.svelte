<script lang="ts">
  import { setContext } from "svelte"
  import type { Snippet } from "svelte"
  import { sidebarPageContext, type SidebarPageState } from "./context"

  let { className = "", children }: { className?: string; children: Snippet } =
    $props()
  const state = $state<SidebarPageState>({
    drawer: undefined,
  })

  setContext(sidebarPageContext, state)
</script>

<div class={`sidebar-page ${className}`}>
  {@render children()}
</div>
