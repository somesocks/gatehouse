<script lang="ts">
  import type { Snippet } from "svelte"

  let { children }: { children: Snippet } = $props()
</script>

<footer class="sidebar-page-sidebar-footer stack">
  {@render children()}
</footer>
