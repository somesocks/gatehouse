<script
  lang="ts"
  generics="T extends { id: string; label: string; disabled?: boolean }"
>
  import { RadioGroup } from "bits-ui"
  import { Bot, Check, ChevronRight, Circle, File, History, KeyRound, ListTodo, NotebookPen, UserRound, UsersRound } from "@lucide/svelte"
  import type { Snippet } from "svelte"
  import Badge from "./Badge.svelte"
  import RouterLink from "./RouterLink.svelte"

  let {
    items,
    label,
    toolbar,
    actions,
    details,
    badges,
    href,
    scrollable = true,
    framed = true,
    plain = false,
    icon,
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
    framed?: boolean
    plain?: boolean
    icon?: "file" | "note" | "task" | "secret" | "person" | "group" | "bot" | "revision"
    emptyMessage?: string
    loading?: boolean
    selectable?: boolean
    value?: string
    disabled?: boolean
    onSelect?: (id: string) => void
  } = $props()
  const rowClass = $derived(plain
    ? "flex min-h-14 w-full items-center gap-3 rounded-md py-2 text-left text-sm hover:bg-muted/50 focus-within:bg-muted/50"
    : "flex min-h-15 w-full items-center gap-4 px-5 py-4 text-left text-sm hover:bg-muted/30 focus-within:bg-muted/30")
  const icons = { file: File, note: NotebookPen, task: ListTodo, secret: KeyRound, person: UserRound, group: UsersRound, bot: Bot, revision: History }
</script>

{#snippet identity(item: T)}
  <span class="flex flex-wrap items-center gap-2">
    <span class={plain ? "font-medium" : undefined}>{item.label}</span>
    {#if !plain}{@render badges?.(item)}{/if}
  </span>
  {#if details}<span
      class="text-muted-foreground line-clamp-2 text-xs leading-relaxed"
      >{@render details(item)}</span
    >{/if}
  {#if item.disabled}<Badge>Disabled</Badge>{/if}
{/snippet}

{#snippet row(item: T, linked = false)}
  {#if plain && icon}
    {@const Icon = icons[icon]}
    <span class="text-muted-foreground bg-muted/60 flex size-9 shrink-0 items-center justify-center rounded-md">
      <Icon size={18} aria-hidden="true" />
    </span>
  {/if}
  <span class="flex min-w-0 flex-1 flex-col gap-1 [overflow-wrap:anywhere]">
    {#if href && !selectable && !linked}
      <RouterLink variant="record" href={href(item)}
        >{@render identity(item)}</RouterLink
      >
    {:else}
      {@render identity(item)}
    {/if}
  </span>
  {#if plain && badges}<span class="flex max-w-[45%] shrink-0 flex-wrap items-center justify-end gap-1">
    {@render badges(item)}
  </span>{/if}
  {#if selectable}<span class="flex shrink-0 items-center justify-end">
      {#if value === item.id}<Check
          size={17}
          aria-hidden="true"
        />{:else}<Circle
          size={17}
          class="text-muted-foreground"
          aria-hidden="true"
        />{/if}
    </span>{:else if actions}<div class={plain ? "flex min-w-0 max-w-[50%] shrink-0 items-center justify-end" : "flex shrink-0 items-center justify-end"}>
      {@render actions?.(item)}
    </div>{/if}
  {#if plain && linked}<ChevronRight class="text-muted-foreground/60 size-4 shrink-0" aria-hidden="true" />{/if}
{/snippet}

<div
  class={plain ? "flex min-w-0 flex-col gap-2" : framed
    ? "min-w-0 overflow-hidden rounded-lg border bg-card"
    : "min-w-0 overflow-hidden"}
>
  {#if toolbar}<div class={plain ? undefined : "border-b p-3"}>{@render toolbar()}</div>{/if}
  <div
    class={scrollable ? "max-h-72 overflow-y-auto" : undefined}
    aria-busy={loading}
  >
    {#if selectable}
      <RadioGroup.Root
        bind:value
        aria-label={label}
        {disabled}
        onValueChange={onSelect}
        class={plain ? "space-y-1" : "divide-y divide-border"}
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
      <ul aria-label={label} class={plain ? "space-y-1" : "divide-y divide-border"}>
        {#each items as item (item.id)}<li class={plain && href && !actions ? undefined : rowClass}>
            {#if plain && href && !actions}
              <RouterLink variant="resource-row" href={href(item)}>{@render row(item, true)}</RouterLink>
            {:else}
              {@render row(item)}
            {/if}
          </li>{/each}
      </ul>
    {/if}
    {#if !items.length && emptyMessage}<p
        class={plain ? "text-muted-foreground py-8 text-sm" : "text-muted-foreground px-5 py-4 text-sm"}
        role="status"
      >
        {emptyMessage}
      </p>{/if}
  </div>
</div>
