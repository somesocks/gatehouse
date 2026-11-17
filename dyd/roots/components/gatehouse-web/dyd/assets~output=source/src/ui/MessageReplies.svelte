<script lang="ts">
  import type { Snippet } from "svelte"
  import type { Attributes } from "./types"
  let { children, ...rest }: Attributes & { children: Snippet } = $props()
</script>

<div
  data-slot="message-replies"
  class="ms-6 flex min-w-0 flex-col gap-3 sm:ms-11"
  {...rest}
>
  {@render children()}
</div>
