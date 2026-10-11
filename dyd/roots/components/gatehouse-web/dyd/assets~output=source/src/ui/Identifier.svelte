<script lang="ts">
  import { Copy, Check } from "@lucide/svelte"
  let { value, copyable = true }: { value: string; copyable?: boolean } =
    $props()
  let copied = $state(false)
  let failed = $state(false)
  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      copied = true
      failed = false
    } catch {
      failed = true
    }
  }
</script>

<span
  class="text-muted-foreground inline-flex max-w-full items-center gap-2 text-xs"
>
  <code class="min-w-0 break-all">{value}</code>
  {#if copyable}<button
      type="button"
      class="hover:bg-muted focus-visible:ring-ring inline-flex shrink-0 items-center rounded p-1 focus-visible:ring-2"
      onclick={copy}
      aria-label={copied ? "ID copied" : "Copy ID"}
      title={copied ? "Copied" : "Copy ID"}
    >
      {#if copied}<Check size={14} />{:else}<Copy size={14} />{/if}
    </button>{/if}
  <span class="sr-only" role="status"
    >{failed ? "Could not copy ID." : copied ? "ID copied." : ""}</span
  >
</span>
