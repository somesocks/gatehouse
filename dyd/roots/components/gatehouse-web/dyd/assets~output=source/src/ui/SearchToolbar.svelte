<script lang="ts">
  import type { Snippet } from "svelte"
  import { Input } from "../lib/vendor/shadcn/input"
  import { Search } from "@lucide/svelte"
  import Button from "./Button.svelte"
  import Spinner from "./Spinner.svelte"
  let {
    value = $bindable(""),
    status = $bindable(""),
    onsearch,
    statuses = false,
    placeholder = "Name, alias, or ID",
    label = "Search records",
    loading = false,
    filters,
    framed = true,
    integrated = false,
  }: {
    value?: string
    status?: string
    onsearch: () => void
    statuses?: boolean
    placeholder?: string
    label?: string
    loading?: boolean
    filters?: Snippet
    framed?: boolean
    integrated?: boolean
  } = $props()
</script>

{#snippet searchField()}
  <div class={integrated ? "relative w-full min-w-0" : "relative min-w-40 flex-1"}>
    <span
      class="text-muted-foreground pointer-events-none absolute top-1/2 left-3 flex size-4 -translate-y-1/2 items-center justify-center"
      aria-hidden="true"
    >
      {#if integrated && loading}<Spinner />{:else}<Search size={16} />{/if}
    </span>
    <Input
      class="h-10 bg-background pl-9"
      type="search"
      aria-label={label}
      aria-busy={integrated ? loading : undefined}
      {placeholder}
      bind:value
    />
  </div>
{/snippet}

<form
  class={integrated
    ? "flex w-full min-w-0 flex-col gap-2"
    : framed
    ? "flex w-full flex-wrap items-center gap-3 rounded-lg border bg-muted/20 p-3 [&_button]:h-10"
    : "flex w-full flex-wrap items-center gap-2 [&_button]:h-10"}
  onsubmit={(event) => {
    event.preventDefault()
    onsearch()
  }}
>
  {#if integrated}
    {@render searchField()}
    {#if filters}<div class="flex w-full flex-wrap items-center gap-3">{@render filters()}</div>{/if}
    <span class="sr-only" role="status">{loading ? "Searching…" : ""}</span>
  {:else}
    {@render searchField()}
    {#if filters}{@render filters()}{/if}
    <Button type="submit" variant="outline" disabled={loading}>Search</Button>
  {/if}
  {#if statuses}<select
      class="border-input bg-background h-10 rounded-md border px-3 text-sm"
      aria-label="Filter by status"
      bind:value={status}
      onchange={onsearch}
      ><option value="">All statuses</option><option value="enabled"
        >Enabled</option
      ><option value="disabled">Disabled</option></select
    >{/if}
</form>
