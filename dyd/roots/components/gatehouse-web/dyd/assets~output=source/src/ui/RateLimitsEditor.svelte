<script lang="ts">
  import Button from "./Button.svelte"
  import JsonInput from "./JsonInput.svelte"
  import Checkbox from "./Checkbox.svelte"
  let { value = $bindable("") }: { value?: string } = $props()
  let advanced = $state(false)
  const scopes = [
    { key: "workspace_input", label: "Workspace input" },
    { key: "workspace_output", label: "Workspace output" },
    { key: "user_input", label: "User input" },
    { key: "user_output", label: "User output" },
  ]
  type Bucket = {
    minimum_balance: number
    maximum_balance: number
    refill_per_minute: number
  }
  const buckets = $derived.by(() => {
    try {
      const parsed = JSON.parse(value || "{}")
      return parsed && typeof parsed === "object" && !Array.isArray(parsed)
        ? (parsed as Record<string, Bucket>)
        : ({} as Record<string, Bucket>)
    } catch {
      return {} as Record<string, Bucket>
    }
  })
  function toggle(key: string, checked: boolean) {
    const next = { ...buckets }
    if (checked)
      next[key] = {
        minimum_balance: -60000,
        maximum_balance: 120000,
        refill_per_minute: 60000,
      }
    else delete next[key]
    value = Object.keys(next).length ? JSON.stringify(next, null, 2) : ""
  }
  function change(key: string, field: keyof Bucket, event: Event) {
    const input = event.currentTarget as HTMLInputElement
    if (input.value === "") return
    value = JSON.stringify(
      { ...buckets, [key]: { ...buckets[key], [field]: Number(input.value) } },
      null,
      2,
    )
  }
</script>

<div class="space-y-4">
  <div class="flex justify-end">
    <Button
      type="button"
      variant="outline"
      size="sm"
      onclick={() => (advanced = !advanced)}
      >{advanced ? "Structured fields" : "Advanced JSON"}</Button
    >
  </div>
  {#if advanced}<label for="rate-limits-json" class="text-sm font-medium"
      >Rate-limit JSON</label
    ><JsonInput id="rate-limits-json" bind:value />{:else}
    <div class="grid gap-4 md:grid-cols-2">
      {#each scopes as scope}<fieldset class="min-w-0 rounded-lg border p-4">
          <legend class="px-1 text-sm font-medium">{scope.label}</legend><label
            class="mb-3 flex items-center gap-2 text-sm"
            ><Checkbox
              aria-label={`${scope.label} rate limit`}
              checked={!!buckets[scope.key]}
              onCheckedChange={(checked) => toggle(scope.key, checked)}
            /> Limit this scope</label
          >
          {#if buckets[scope.key]}<div class="grid gap-3 sm:grid-cols-3">
              {#each [{ key: "minimum_balance", label: "Minimum balance" }, { key: "maximum_balance", label: "Maximum balance" }, { key: "refill_per_minute", label: "Refill per minute" }] as field}<label
                  class="space-y-1 text-xs"
                  ><span class="text-muted-foreground">{field.label}</span
                  ><input
                    type="number"
                    step="any"
                    value={buckets[scope.key][field.key as keyof Bucket]}
                    class="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
                    oninput={(event) =>
                      change(scope.key, field.key as keyof Bucket, event)}
                  /></label
                >{/each}
            </div>{/if}
        </fieldset>{/each}
    </div>
  {/if}
</div>
