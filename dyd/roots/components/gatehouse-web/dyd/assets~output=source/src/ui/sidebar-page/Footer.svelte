<script lang="ts">
  import type { Snippet } from "svelte"
  import { cn } from "../../lib/utils"
  let { children, compact = false }: { children: Snippet; compact?: boolean } = $props()
</script>

<footer
  class={cn("sticky bottom-0 z-20 shrink-0 border-t bg-card p-4 md:rounded-b-xl md:px-6", compact && "p-2 pb-[max(0.5rem,env(safe-area-inset-bottom))] md:p-4 md:px-6")}
>
  {@render children()}
</footer>
