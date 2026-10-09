<script lang="ts">
  import type { Attributes } from "./types"
  let { children, ...rest }: Attributes = $props()
</script>

<div data-slot="feed" class="flex w-full min-w-0 flex-col gap-6" {...rest}>
  {@render children?.()}
</div>
