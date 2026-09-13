<script lang="ts">
  import type { Snippet } from "svelte"

  let { children }: { children: Snippet } = $props()
</script>

<main class="sidebar-page-page">{@render children()}</main>
