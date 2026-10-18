<script lang="ts">
  import * as UI from "../../ui"
  import { Bot, UserRound, UsersRound } from "../../ui"
  import { tick } from "svelte"
  import type { MessageTarget, MessageTargetRef } from "../../app/chat"
  import { targetKey, targetLabel } from "./message-targets"

  let {
    open = $bindable(false),
    options,
    ready,
    error,
    onselect,
    onretry,
    onclose,
  }: {
    open?: boolean
    options: MessageTarget[]
    ready: boolean
    error: string
    onselect: (ref: MessageTargetRef | null) => void
    onretry: () => void
    onclose: () => void
  } = $props()
  let query = $state("")
  let searchElement = $state<HTMLInputElement>()
  const kinds = {
    agent: { label: "Bot", icon: Bot },
    principal: { label: "Person", icon: UserRound },
    group: { label: "Group", icon: UsersRound },
  } as const
  const filtered = $derived(options
    .filter((target) => `${target.alias ?? ""} ${target.name ?? ""}`.toLowerCase().includes(query.trim().toLowerCase()))
    .sort((a, b) => targetLabel(a).localeCompare(targetLabel(b))))
</script>

<UI.Dialog.Root bind:open>
  <UI.Dialog.Content
    preventScroll={false}
    onOpenAutoFocus={(event) => {
      event.preventDefault()
      query = ""
      void tick().then(() => searchElement?.focus({ preventScroll: true }))
    }}
    onCloseAutoFocus={(event) => { event.preventDefault(); onclose() }}
  >
    <UI.DialogTitle>Choose recipient</UI.DialogTitle>
    <UI.Command.Root label="Choose recipient" shouldFilter={false}>
      <UI.Command.Input bind:ref={searchElement} bind:value={query} placeholder="Search bots, people, and groups" />
      <UI.Command.List aria-busy={!ready && error === ""}>
        <UI.Command.Group>
          <UI.Command.Item
            value="none"
            onSelect={() => {
              onselect(null)
              open = false
            }}
          >
            <UI.X size={16} visibility="hidden" aria-hidden="true" />
            <UI.Text truncate>@none</UI.Text>
            <UI.Text truncate size="small" tone="muted">clear selected</UI.Text>
          </UI.Command.Item>
          {#each filtered as target (targetKey(target.ref))}
            {@const kind = kinds[target.ref.kind]}
            <UI.Command.Item
              value={targetKey(target.ref)}
              aria-label={`${targetLabel(target)}, ${kind.label}${target.alias && target.name ? `, ${target.name}` : ""}`}
              disabled={!ready || error !== ""}
              onSelect={() => {
                onselect(target.ref)
                open = false
              }}
            >
              <kind.icon size={16} aria-hidden="true" />
              <UI.Text truncate title={targetLabel(target)}>{targetLabel(target)}</UI.Text>
              {#if target.alias && target.name}<UI.Text truncate size="small" tone="muted" title={target.name}>{target.name}</UI.Text>{/if}
            </UI.Command.Item>
          {/each}
        </UI.Command.Group>
        {#if ready && filtered.length === 0}<UI.Text as="p" tone="muted">No recipients found.</UI.Text>{/if}
      </UI.Command.List>
    </UI.Command.Root>
    {#if error}<UI.Text as="p" role="alert">{error}</UI.Text><UI.Button type="button" variant="outline" onclick={onretry}>Try again</UI.Button>{/if}
  </UI.Dialog.Content>
</UI.Dialog.Root>
