<script lang="ts">
  import type { Snippet } from "svelte"

  let { children }: { children: Snippet } = $props()
</script>

<div class="page-body">
  {@render children()}
</div>

<style>
  .page-body {
    display: grid;
    gap: 1rem;
  }
</style>
