<script lang="ts">
  import type { Snippet } from "svelte"

  let {
    fluid = false,
    children,
  }: { fluid?: boolean; children: Snippet } = $props()
</script>

<div class={`stack${fluid ? " page-content" : " container"}`}>
  {@render children()}
</div>
