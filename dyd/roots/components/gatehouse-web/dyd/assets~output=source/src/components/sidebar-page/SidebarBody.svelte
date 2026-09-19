<script lang="ts">
  import type { Snippet } from "svelte"

  let { children }: { children: Snippet } = $props()
</script>

<section class="sidebar-page-sidebar-body stack">
  {@render children()}
</section>
