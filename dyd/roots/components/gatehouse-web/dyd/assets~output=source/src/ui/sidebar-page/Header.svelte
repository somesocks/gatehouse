<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    placement = "inline",
    children,
  }: { placement?: "inline" | "floating"; children: Snippet } = $props()
</script>

<header
  class="sticky top-0 z-20 flex h-16 shrink-0 items-center bg-background md:rounded-t-xl"
>
  <div class="flex w-full min-w-0 items-center gap-2 px-4">
    {@render children()}
  </div>
</header>
