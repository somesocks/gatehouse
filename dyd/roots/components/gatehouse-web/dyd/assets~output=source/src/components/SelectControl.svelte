<script lang="ts">
  import { ChevronDown } from "@lucide/svelte"
  import type { Snippet } from "svelte"

  let {
    children,
    trailing = false,
  }: {
    children: Snippet
    trailing?: boolean
  } = $props()
</script>

<span class="select-control" data-trailing={trailing || undefined}>
  {@render children()}
  <ChevronDown size={16} strokeWidth={2} aria-hidden="true" />
</span>
