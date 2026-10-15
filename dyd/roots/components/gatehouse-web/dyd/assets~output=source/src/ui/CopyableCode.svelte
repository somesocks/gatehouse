<script lang="ts">
  import { Copy } from "@lucide/svelte"
  import Button from "./Button.svelte"

  let {
    value,
    oncopy,
  }: {
    value: string
    oncopy: () => void
  } = $props()
</script>

<div
  class="flex min-w-0 items-center justify-center gap-2 py-4"
  role="group"
  aria-label="One-time code"
>
  <code
    class="min-w-0 select-all break-all font-mono text-3xl font-semibold tracking-wider"
    >{value}</code
  >
  <Button
    variant="ghost"
    size="icon"
    aria-label="Copy code"
    title="Copy code"
    onclick={oncopy}
  >
    <Copy aria-hidden="true" />
  </Button>
</div>
