<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    children,
  }: { fluid?: boolean; width?: "wide" | "form"; children: Snippet } = $props()
</script>

<div class="flex w-full min-w-0 flex-col gap-6">
  {@render children()}
</div>
