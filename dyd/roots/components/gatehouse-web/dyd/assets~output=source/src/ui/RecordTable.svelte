<script lang="ts">
  import type { Snippet } from "svelte"
  let { headers, children }: { headers: string[]; children: Snippet } = $props()
</script>

<div class="w-full overflow-x-auto rounded-lg border bg-card">
  <table class="w-full min-w-[40rem] border-collapse text-left text-sm">
    <thead class="border-b bg-muted/40 text-muted-foreground"
      ><tr
        >{#each headers as header}<th
            scope="col"
            class="h-11 px-5 py-3 text-xs font-medium tracking-wide whitespace-nowrap last:text-right"
            >{header}</th
          >{/each}</tr
      ></thead
    >
    <tbody class="divide-y divide-border">{@render children()}</tbody>
  </table>
</div>
