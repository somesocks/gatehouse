<script lang="ts">
  import type { Attributes } from "./types"
  let { html, size = "small", ...rest }: Attributes & { html: string; size?: "small" | "normal" } = $props()
</script>

<div
  class={`prose ${size === "normal" ? "prose-base" : "prose-sm"} dark:prose-invert max-w-none min-w-0 break-words`}
  {...rest}
>
  {@html html}
</div>
