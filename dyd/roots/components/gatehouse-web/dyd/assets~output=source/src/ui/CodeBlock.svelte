<script lang="ts">
  let { value, json = false }: { value: string; json?: boolean } = $props()
  const formatted = $derived.by(() => {
    if (!json) return value
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return value
    }
  })
</script>

<pre
  class="bg-muted/50 max-h-96 overflow-auto rounded-md border p-4 text-xs leading-relaxed whitespace-pre-wrap break-words"><code
    >{formatted || "Not configured"}</code
  ></pre>
