<script lang="ts">
  import type { Attributes } from "./types"
  let { children, ...rest }: Attributes = $props()
</script>

<div
  data-slot="feed"
  class="flex min-h-full w-full min-w-0 flex-1 flex-col gap-6 bg-muted/40 p-6"
  {...rest}
>
  {@render children?.()}
</div>
