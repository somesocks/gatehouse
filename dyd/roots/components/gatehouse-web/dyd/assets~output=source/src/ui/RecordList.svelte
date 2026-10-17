<script
  lang="ts"
  generics="T extends { id: string; label: string; disabled?: boolean }"
>
  import { RadioGroup } from "bits-ui"
  import { Check, Circle } from "@lucide/svelte"
  import type { Snippet } from "svelte"
  import Badge from "./Badge.svelte"
  import RouterLink from "./RouterLink.svelte"
  import { resourceDialog } from "./resource-dialog-context"

  let {
    items,
    label,
    toolbar,
    actions,
    details,
    badges,
    href,
    scrollable = true,
    pageKey,
    emptyMessage = "No matches.",
    loading = false,
    selectable = false,
    value = $bindable(""),
    disabled = false,
    onSelect,
  }: {
    items: T[]
    label: string
    toolbar?: Snippet
    actions?: Snippet<[T]>
    details?: Snippet<[T]>
    badges?: Snippet<[T]>
    href?: (item: T) => string
    scrollable?: boolean
    pageKey?: string
    emptyMessage?: string
    loading?: boolean
    selectable?: boolean
    value?: string
    disabled?: boolean
    onSelect?: (id: string) => void
  } = $props()
  const rowClass =
    "flex min-h-15 w-full items-center gap-4 px-5 py-4 text-left text-sm hover:bg-muted/30 focus-within:bg-muted/30"
  const dialog = resourceDialog()
  let previousPage: string | undefined
  $effect(() => {
    const key = pageKey
    if (previousPage !== undefined && previousPage !== key) dialog?.resetScroll()
    previousPage = key
  })
</script>

{#snippet identity(item: T)}
  <span class="flex flex-wrap items-center gap-2">
    <span>{item.label}</span>
    {@render badges?.(item)}
  </span>
  {#if details}<span class="text-muted-foreground line-clamp-2 text-xs leading-relaxed">{@render details(item)}</span>{/if}
  {#if item.disabled}<Badge>Disabled</Badge>{/if}
{/snippet}

{#snippet row(item: T)}
  <span class="flex min-w-0 flex-1 flex-col gap-1 [overflow-wrap:anywhere]">
    {#if href && !selectable}
      <RouterLink variant="record" href={href(item)}>{@render identity(item)}</RouterLink>
    {:else}
      {@render identity(item)}
    {/if}
  </span>
  {#if selectable}<span class="flex shrink-0 items-center justify-end">
      {#if value === item.id}<Check
          size={17}
          aria-hidden="true"
        />{:else}<Circle
          size={17}
          class="text-muted-foreground"
          aria-hidden="true"
        />{/if}
    </span>{:else}<div class="flex shrink-0 items-center justify-end">
      {@render actions?.(item)}
    </div>{/if}
{/snippet}

<div class="min-w-0 overflow-hidden rounded-lg border bg-card">
  {#if toolbar}<div class="border-b p-3">{@render toolbar()}</div>{/if}
  <div class={scrollable ? "max-h-72 overflow-y-auto" : undefined} aria-busy={loading}>
    {#if selectable}
      <RadioGroup.Root
        bind:value
        aria-label={label}
        {disabled}
        onValueChange={onSelect}
        class="divide-y divide-border"
      >
        {#each items as item (item.id)}
          <RadioGroup.Item
            value={item.id}
            aria-label={item.label}
            disabled={item.disabled}
            class={`${rowClass} data-[state=checked]:bg-muted/60 focus-visible:ring-ring outline-none focus-visible:ring-2 focus-visible:ring-inset disabled:opacity-50`}
          >
            {@render row(item)}
          </RadioGroup.Item>
        {/each}
      </RadioGroup.Root>
    {:else}
      <ul aria-label={label} class="divide-y divide-border">
        {#each items as item (item.id)}<li class={rowClass}>
            {@render row(item)}
          </li>{/each}
      </ul>
    {/if}
    {#if !items.length}<p
        class="text-muted-foreground px-5 py-4 text-sm"
        role="status"
      >
        {emptyMessage}
      </p>{/if}
  </div>
</div>
