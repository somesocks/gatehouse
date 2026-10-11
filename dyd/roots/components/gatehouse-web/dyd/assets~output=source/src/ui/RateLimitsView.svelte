<script lang="ts">
  type Bucket = {
    minimum_balance: number
    maximum_balance: number
    refill_per_minute: number
  }
  let {
    value,
  }: {
    value?: {
      workspace_input?: Bucket
      workspace_output?: Bucket
      user_input?: Bucket
      user_output?: Bucket
    }
  } = $props()
  const rows = $derived(
    Object.entries(value ?? {}).filter(
      (entry): entry is [string, Bucket] => !!entry[1],
    ),
  )
</script>

{#if rows.length}<div class="overflow-auto">
    <table class="w-full text-left text-xs">
      <thead class="text-muted-foreground"
        ><tr
          ><th class="pb-3 pr-4 font-medium">Scope</th><th
            class="pb-3 pr-4 text-right font-medium">Minimum</th
          ><th class="pb-3 pr-4 text-right font-medium">Maximum</th><th
            class="pb-3 text-right font-medium">Refill/min</th
          ></tr
        ></thead
      ><tbody
        >{#each rows as [scope, bucket]}<tr class="border-t"
            ><td class="py-3 pr-4 capitalize">{scope.replaceAll("_", " ")}</td
            ><td class="py-3 pr-4 text-right tabular-nums"
              >{bucket.minimum_balance.toLocaleString()}</td
            ><td class="py-3 pr-4 text-right tabular-nums"
              >{bucket.maximum_balance.toLocaleString()}</td
            ><td class="py-3 text-right tabular-nums"
              >{bucket.refill_per_minute.toLocaleString()}</td
            ></tr
          >{/each}</tbody
      >
    </table>
  </div>{:else}<p class="text-muted-foreground">Not configured</p>{/if}
