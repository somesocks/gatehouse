<script lang="ts">
  import { ChevronDown } from "@lucide/svelte"
  import type { Snippet } from "svelte"

  let {
    children,
    clearable = false,
  }: {
    children: Snippet
    clearable?: boolean
  } = $props()
</script>

<span class="select-control" data-clearable={clearable || undefined}>
  {@render children()}
  <ChevronDown size={16} strokeWidth={2} aria-hidden="true" />
</span>
