<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    mode = "collection",
    children,
  }: {
    mode?: "collection" | "detail" | "form" | "hub"
    children: Snippet
  } = $props()
</script>

<section class="flex w-full min-w-0 flex-col gap-6" data-page-purpose={mode}>
  {@render children()}
</section>
