<script lang="ts">
  import * as UI from "../../ui"
  import { Bot, UserRound, UsersRound } from "../../ui"
  import { tick } from "svelte"
  import type { MessageTarget, MessageTargetRef } from "../../app/chat"
  import { targetKey, targetLabel } from "./message-targets"

  let {
    open = $bindable(false),
    inChat,
    outside,
    canManage,
    ready,
    error,
    outsideReady,
    outsideError,
    onselect,
    onretry,
    onclose,
  }: {
    open?: boolean
    inChat: MessageTarget[]
    outside: MessageTarget[]
    canManage: boolean
    ready: boolean
    error: string
    outsideReady: boolean
    outsideError: string
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
  const lists = $derived([
    { label: "In this chat", targets: inChat, ready, error: "" },
    ...(canManage ? [{ label: "Outside this chat", targets: outside, ready: outsideReady, error: outsideError }] : []),
  ].map((list) => ({ ...list, targets: list.targets
    .filter((target) => `${target.alias ?? ""} ${target.name ?? ""}`.toLowerCase().includes(query.trim().toLowerCase()))
    .sort((a, b) => targetLabel(a).localeCompare(targetLabel(b))) })))
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
            <UI.Text truncate>@none</UI.Text>
            <UI.Text truncate size="small" tone="muted">clear selected</UI.Text>
          </UI.Command.Item>
        </UI.Command.Group>
        {#each lists as list (list.label)}
          <UI.Command.Group heading={list.label} aria-busy={!list.ready && list.error === ""}>
          {#each list.targets as target (targetKey(target.ref))}
            {@const kind = kinds[target.ref.kind]}
            <UI.Command.Item
              value={targetKey(target.ref)}
              aria-label={`${targetLabel(target)}, ${kind.label}${target.alias && target.name ? `, ${target.name}` : ""}`}
              disabled={!ready || error !== "" || !list.ready || list.error !== ""}
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
          {#if list.ready && list.targets.length === 0}<UI.Text as="p" size="small" tone="muted">No recipients found.</UI.Text>{/if}
          {#if list.error}
            <UI.Row>
              <UI.Text as="p" size="small" role="alert">{list.error}</UI.Text>
              <UI.Button type="button" variant="outline" size="sm" onclick={onretry}>Try again</UI.Button>
            </UI.Row>
          {/if}
          </UI.Command.Group>
        {/each}
      </UI.Command.List>
    </UI.Command.Root>
    {#if error}<UI.Text as="p" role="alert">{error}</UI.Text><UI.Button type="button" variant="outline" onclick={onretry}>Try again</UI.Button>{/if}
  </UI.Dialog.Content>
</UI.Dialog.Root>
