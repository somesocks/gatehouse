<script lang="ts">
  import { tick } from "svelte"
  import type { Group } from "../../app/groups"
  import * as UI from "../../ui"

  let {
    open = $bindable(false),
    groups,
    selected,
    loading,
    error,
    onselect,
    onretry,
    onclose,
  }: {
    open?: boolean
    groups: Group[]
    selected: string
    loading: boolean
    error: string
    onselect: (id: string) => void
    onretry: () => void
    onclose: () => void
  } = $props()
  let query = $state("")
  let searchElement = $state<HTMLInputElement>()
  const matches = $derived(
    groups
      .filter((group) =>
        `${group.name ?? ""} ${group.alias ?? ""}`
          .toLowerCase()
          .includes(query.trim().toLowerCase()),
      )
      .sort((a, b) =>
        (a.name ?? a.alias ?? a.id).localeCompare(b.name ?? b.alias ?? b.id),
      ),
  )
  function select(id: string): void {
    onselect(id)
    open = false
  }
</script>

<UI.Dialog.Root bind:open>
  <UI.Dialog.Content
    preventScroll={false}
    onOpenAutoFocus={(event) => {
      event.preventDefault()
      query = ""
      void tick().then(() => searchElement?.focus({ preventScroll: true }))
    }}
    onCloseAutoFocus={(event) => {
      event.preventDefault()
      onclose()
    }}
  >
    <UI.DialogTitle>Send as</UI.DialogTitle>
    <UI.Command.Root label="Send as" shouldFilter={false}>
      <UI.Command.Input
        bind:ref={searchElement}
        bind:value={query}
        placeholder="Search your groups"
      />
      <UI.Command.List aria-busy={loading}>
        <UI.Command.Group>
          <UI.Command.Item value="clear" onSelect={() => select("")}>
            <UI.Text>Clear</UI.Text>
            <UI.Text size="small" tone="muted">Send as yourself</UI.Text>
          </UI.Command.Item>
        </UI.Command.Group>
        <UI.Command.Group heading="Your groups">
          {#if loading}
            <UI.Text as="p" size="small" tone="muted">Loading groups…</UI.Text>
          {:else if !error}
            {#each matches as group (group.id)}
              <UI.Command.Item
                value={group.id}
                onSelect={() => select(group.id)}
              >
                <UI.UsersRound size={16} aria-hidden="true" />
                <UI.Text truncate
                  >{group.name ?? group.alias ?? group.id}</UI.Text
                >
                {#if group.name && group.alias}
                  <UI.Text truncate size="small" tone="muted"
                    >@{group.alias}</UI.Text
                  >
                {/if}
                {#if group.id === selected}
                  <UI.Text size="small" tone="muted">Selected</UI.Text>
                {/if}
              </UI.Command.Item>
            {:else}
              <UI.Text as="p" size="small" tone="muted"
                >No matching groups.</UI.Text
              >
            {/each}
          {/if}
        </UI.Command.Group>
      </UI.Command.List>
    </UI.Command.Root>
    {#if error}
      <UI.Text as="p" role="alert">{error}</UI.Text>
      <UI.Button type="button" variant="outline" onclick={onretry}
        >Try again</UI.Button
      >
    {/if}
  </UI.Dialog.Content>
</UI.Dialog.Root>
