<script lang="ts">
  import * as Dialog from "./dialog"
  import Input from "./Input.svelte"
  import Button from "./Button.svelte"
  import { Search, Check, Circle, X } from "@lucide/svelte"
  import { createReferenceSelection } from "./reference-selection.svelte"
  let {
    value = $bindable(""),
    search = $bindable(""),
    label,
    compact = false,
    showIdentifiers = true,
    placeholder,
    selectedLabel,
    items,
    loading = false,
    error = "",
    previous = false,
    next = false,
    onsearch,
    onprevious,
    onnext,
    onclose,
  }: {
    value?: string
    search?: string
    label: string
    compact?: boolean
    showIdentifiers?: boolean
    placeholder?: string
    selectedLabel?: string
    items: {
      id: string
      label: string
      description?: string
      disabled?: boolean
    }[]
    loading?: boolean
    error?: string
    previous?: boolean
    next?: boolean
    onsearch: () => void
    onprevious: () => void
    onnext: () => void
    onclose?: () => void
  } = $props()
  const uid = $props.id()
  const selection = createReferenceSelection()
  let open = $state(false)
  let field = $state<HTMLButtonElement>()
  let searchField = $state<HTMLInputElement>()
  let results = $state<HTMLDivElement>()
  const displayName = $derived(
    selectedLabel ??
      items.find((item) => item.id === value)?.label ??
      (selection.state.initial === value
        ? selection.state.initialLabel
        : undefined) ??
      (showIdentifiers ? value : `Selected ${label.toLowerCase()}`),
  )
  const firstSelectable = $derived(items.find((item) => !item.disabled)?.id)
  const draftOnPage = $derived(
    items.some((item) => item.id === selection.state.draft && !item.disabled),
  )
  function openPicker() {
    selection.begin(value, displayName)
    open = true
    onsearch()
  }
  function closePicker() {
    selection.cancel()
    open = false
    onclose?.()
  }
  function confirm() {
    if (!selection.state.draft || loading) return
    value = selection.confirm()
    open = false
    onclose?.()
  }
  function navigateResults(event: KeyboardEvent) {
    if (!["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return
    const buttons = Array.from(
      results?.querySelectorAll<HTMLButtonElement>(
        'button[role="radio"]:not(:disabled)',
      ) ?? [],
    )
    if (!buttons.length) return
    event.preventDefault()
    const current = buttons.indexOf(event.currentTarget as HTMLButtonElement)
    const index =
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? buttons.length - 1
          : (current + (event.key === "ArrowDown" ? 1 : -1) + buttons.length) %
            buttons.length
    buttons[index].focus()
    buttons[index].click()
  }
</script>

<Dialog.Root
  bind:open
  onOpenChange={(active) => {
    if (!active) {
      selection.cancel()
      onclose?.()
    }
  }}
>
  <div class="min-w-0 space-y-2">
    <label
      id={`${uid}-label`}
      for={`${uid}-reference`}
      class={compact ? "sr-only" : "text-sm font-medium"}>{label}</label
    >
    <div
      class="border-input bg-background focus-within:ring-ring flex min-w-0 items-center rounded-md border focus-within:ring-2"
    >
      <button
        bind:this={field}
        id={`${uid}-reference`}
        type="button"
        aria-labelledby={`${uid}-label ${uid}-value`}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={`${uid}-dialog`}
        class="flex h-10 min-w-0 flex-1 cursor-pointer items-center gap-3 rounded-md bg-transparent px-3 text-left text-sm outline-none"
        onclick={openPicker}
        onkeydown={(event) => {
          if (event.key === "ArrowDown") {
            event.preventDefault()
            openPicker()
          }
        }}
      >
        <span
          id={`${uid}-value`}
          class={value
            ? "min-w-0 flex-1 truncate"
            : "text-muted-foreground min-w-0 flex-1 truncate"}
        >
          {value
            ? displayName
            : (placeholder ?? `Select ${label.toLowerCase()}…`)}
        </span>
        <Search
          class="text-muted-foreground shrink-0"
          size={17}
          aria-hidden="true"
        />
      </button>
      {#if value}<button
          type="button"
          aria-label={`Clear ${label.toLowerCase()} selection`}
          title="Clear selection"
          class="text-muted-foreground hover:text-foreground focus-visible:ring-ring mr-1 shrink-0 rounded p-2 focus-visible:ring-2"
          onclick={() => {
            value = ""
            selection.begin("", "")
            field?.focus()
          }}><X size={16} aria-hidden="true" /></button
        >{/if}
    </div>
    {#if value && !compact && showIdentifiers}<p
        class="text-muted-foreground font-mono text-xs break-all"
      >
        {value}
      </p>{/if}
  </div>

  <Dialog.Content
    id={`${uid}-dialog`}
    size="selector"
    showCloseButton
    onOpenAutoFocus={(event) => {
      event.preventDefault()
      searchField?.focus()
    }}
    onCloseAutoFocus={(event) => {
      event.preventDefault()
      field?.focus()
    }}
  >
    <div class="shrink-0 border-b px-6 py-5 pr-12">
      <Dialog.Title>Select {label.toLowerCase()}</Dialog.Title>
      <Dialog.Description
        >Search and select an existing {label.toLowerCase()}. Confirm to update
        the reference.</Dialog.Description
      >
    </div>
    <div class="flex shrink-0 gap-3 border-b bg-muted/20 px-6 py-4">
      <Input
        bind:ref={searchField}
        type="search"
        aria-label={`Search ${label.toLowerCase()}`}
        placeholder={showIdentifiers ? "Name, alias, or ID" : "Search by name"}
        bind:value={search}
        onkeydown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault()
            onsearch()
          }
        }}
      />
      <Button
        type="button"
        variant="outline"
        disabled={loading}
        onclick={onsearch}>Search</Button
      >
    </div>
    <div
      class="min-h-0 min-w-0 flex-1 overflow-auto"
      bind:this={results}
      role="radiogroup"
      aria-label={`${label} search results`}
      aria-busy={loading}
    >
      {#if error}<p class="text-destructive p-6 text-sm" role="alert">
          {error}
        </p>{/if}
      {#if loading && !items.length}<p
          class="text-muted-foreground p-8 text-center text-sm"
          role="status"
        >
          Searching…
        </p>
      {:else if items.length}<table class="w-full text-left text-sm">
          <thead
            class="text-muted-foreground sticky top-0 border-b bg-card text-xs"
            ><tr
              ><th scope="col" class="px-6 py-3 font-medium">{label}</th><th
                scope="col"
                class="px-6 py-3 text-right font-medium">Selection</th
              ></tr
            ></thead
          >
          <tbody class="divide-y">
            {#each items as item (item.id)}<tr
                class={selection.state.draft === item.id
                  ? "bg-muted/60"
                  : "hover:bg-muted/30"}
              >
                <td colspan="2" class="min-w-0 p-0">
                  <button
                    type="button"
                    role="radio"
                    aria-checked={selection.state.draft === item.id}
                    disabled={item.disabled}
                    tabindex={item.id === selection.state.draft ||
                    (!draftOnPage && item.id === firstSelectable)
                      ? 0
                      : -1}
                    class="focus-visible:ring-ring flex w-full items-center justify-between gap-4 px-6 py-4 text-left focus-visible:ring-2 focus-visible:ring-inset disabled:opacity-50"
                    onclick={() => selection.choose(item.id, item.label)}
                    onkeydown={navigateResults}
                  >
                    <span class="min-w-0 flex-1">
                      <span class="block font-medium">{item.label}</span>
                      {#if item.description}<span
                          class="text-muted-foreground mt-1 block text-xs"
                          >{item.description}</span
                        >{/if}
                      {#if showIdentifiers}<span
                          class="text-muted-foreground mt-1 block font-mono text-xs break-all"
                          >{item.id}</span
                        >{/if}
                      {#if item.disabled}<span
                          class="text-muted-foreground mt-1 block text-xs"
                          >Disabled</span
                        >{/if}
                    </span>
                    <span class="shrink-0" aria-hidden="true"
                      >{#if selection.state.draft === item.id}<Check
                          size={18}
                        />{:else}<Circle
                          class="text-muted-foreground"
                          size={16}
                        />{/if}</span
                    >
                  </button>
                </td>
              </tr>{/each}
          </tbody>
        </table>{:else if !error}<p
          class="text-muted-foreground p-8 text-center text-sm"
        >
          No matches. Try another search.
        </p>{/if}
    </div>
    <div
      class="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t px-6 py-3"
    >
      <p class="text-muted-foreground text-xs">
        {loading ? "Loading results" : `${items.length} results on this page`}
      </p>
      <div class="flex gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!previous || loading}
          onclick={onprevious}>Previous</Button
        ><Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!next || loading}
          onclick={onnext}>Next</Button
        >
      </div>
    </div>
    <div
      class="flex shrink-0 flex-wrap items-center justify-between gap-4 border-t bg-muted/20 px-6 py-4"
    >
      <p class="text-muted-foreground min-w-0 flex-1 text-xs">
        {selection.state.draft
          ? `Selected: ${selection.state.label}`
          : "Select a row to continue."}
      </p>
      <div class="flex gap-2">
        <Button type="button" variant="outline" onclick={closePicker}
          >Cancel</Button
        ><Button
          type="button"
          disabled={!selection.state.draft || loading}
          onclick={confirm}>Use {label.toLowerCase()}</Button
        >
      </div>
    </div>
  </Dialog.Content>
</Dialog.Root>
