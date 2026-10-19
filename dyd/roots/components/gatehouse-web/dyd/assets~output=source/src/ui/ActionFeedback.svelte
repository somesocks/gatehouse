<script lang="ts">
  let {
    error = "",
    notice = "",
    pending = false,
    label = "Saving changes",
  }: {
    error?: string
    notice?: string
    pending?: boolean
    label?: string
  } = $props()
</script>

<div class="min-h-5 text-sm" aria-live="polite" aria-atomic="true">
  {#if error}<p class="text-destructive" role="alert">{error}</p>
  {:else}<p class="text-muted-foreground">{pending ? label : notice}</p>{/if}
</div>
