<script lang="ts">
  import type { Snippet } from "svelte"
  import { Input } from "../lib/vendor/shadcn/input"
  import { Search } from "@lucide/svelte"
  import Button from "./Button.svelte"
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
  } = $props()
</script>

<form
  class={framed
    ? "flex w-full flex-wrap items-center gap-3 rounded-lg border bg-muted/20 p-3 [&_button]:h-10"
    : "flex w-full flex-wrap items-center gap-2 [&_button]:h-10"}
  onsubmit={(event) => {
    event.preventDefault()
    onsearch()
  }}
>
  <div class="relative min-w-40 flex-1">
    <Search
      class="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2"
      aria-hidden="true"
    />
    <Input
      class="h-10 bg-background pl-9"
      type="search"
      aria-label={label}
      {placeholder}
      bind:value
    />
  </div>
  {#if filters}{@render filters()}{/if}
  <Button type="submit" variant="outline" disabled={loading}>Search</Button>
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
