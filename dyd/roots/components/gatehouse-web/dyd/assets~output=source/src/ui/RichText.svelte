<script lang="ts">
  import type { Attributes } from "./types"
  let { html, ...rest }: Attributes & { html: string } = $props()
</script>

<div
  class="prose prose-sm dark:prose-invert max-w-none min-w-0 break-words"
  {...rest}
>
  {@html html}
</div>
