<script lang="ts">
  import { MediaQuery } from "svelte/reactivity"
  import { Ellipsis, Files, KeyRound, ListTodo, Menu as MenuIcon, NotebookPen, Users } from "@lucide/svelte"
  import * as Menu from "./dropdown-menu"
  import { Button } from "../lib/vendor/shadcn/button"
  import { Input } from "../lib/vendor/shadcn/input"
  import RouterLink from "./RouterLink.svelte"
  import Row from "./Row.svelte"
  import Modal from "./Modal.svelte"
  import Form from "./Form.svelte"
  import Field from "./Field.svelte"
  import Label from "./Label.svelte"
  import Notice from "./Notice.svelte"
  import type { SessionNavigationItem } from "./navigation"
  import type { DialogHandle } from "./types"
  let {
    items,
    name = "",
    onRename,
    onParticipants,
    participantsOpen = false,
  }: {
    items: SessionNavigationItem[]
    name?: string
    onRename?: (name: string) => Promise<void>
    onParticipants?: () => void
    participantsOpen?: boolean
  } = $props()
  let modal = $state<DialogHandle>()
  let draft = $state("")
  let error = $state("")
  let saving = $state(false)
  const icons = { files: Files, notes: NotebookPen, tasks: ListTodo, secrets: KeyRound }
  const mobile = new MediaQuery("(max-width: 767px)", false)
  function edit() {
    draft = name
    error = ""
    modal?.showModal()
  }
  async function save() {
    if (saving || !onRename) return
    saving = true
    error = ""
    try {
      await onRename(draft)
      modal?.close()
    } catch (reason) {
      error =
        reason instanceof Error
          ? reason.message
          : "The name could not be saved. Try again."
    } finally {
      saving = false
    }
  }
</script>

<nav class="ml-auto flex shrink-0 items-center gap-1" aria-label="Chat tools">
  {#if mobile.current}
    <Menu.Root>
      <Menu.Trigger aria-label="Chat tools" title="Chat tools">
        <MenuIcon aria-hidden="true" />
      </Menu.Trigger>
      <Menu.Content align="end">
        {#each items as item (item.href)}
          {#if item.icon}
            {@const Icon = icons[item.icon]}
            <Menu.Item>
              {#snippet child({ props })}
                <RouterLink {...props} href={item.href} aria-haspopup="dialog">
                  <Icon size={16} aria-hidden="true" />
                  {item.label}
                </RouterLink>
              {/snippet}
            </Menu.Item>
          {/if}
        {/each}
        {#if onParticipants}
          <Menu.Item onSelect={onParticipants}>
            <Users size={16} aria-hidden="true" />
            Participants
          </Menu.Item>
        {/if}
        {#if onRename}
          <Menu.Separator />
          <Menu.Item onSelect={edit}>Rename</Menu.Item>
        {/if}
      </Menu.Content>
    </Menu.Root>
  {:else}
    {#each items as item (item.href)}
      {#if item.icon}
        {@const Icon = icons[item.icon]}
        <RouterLink
          variant={item.active ? "outline" : "ghost"}
          size="icon"
          href={item.href}
          aria-label={item.label}
          title={item.label}
          aria-haspopup="dialog"
          aria-expanded={item.active === true}
        ><Icon size={18} aria-hidden="true" /></RouterLink>
      {/if}
    {/each}
    {#if onParticipants}
      <Button variant={participantsOpen ? "outline" : "ghost"} size="icon" aria-label="Participants" title="Participants" aria-haspopup="dialog" aria-expanded={participantsOpen} onclick={onParticipants}>
        <Users size={18} aria-hidden="true" />
      </Button>
    {/if}
    {#if onRename}
      <Menu.Root>
        <Menu.Trigger aria-label="More chat actions" title="More chat actions"><Ellipsis aria-hidden="true" /></Menu.Trigger>
        <Menu.Content align="end">
          <Menu.Item onSelect={edit}>Rename</Menu.Item>
        </Menu.Content>
      </Menu.Root>
    {/if}
  {/if}
</nav>
<Modal
  bind:modal
  title="Rename"
  oncancel={(event) => {
    if (saving) event.preventDefault()
  }}
>
  <Form
    onsubmit={(event) => {
      event.preventDefault()
      void save()
    }}
  >
    <Field
      ><Label for="rename-name">Name</Label><Input
        id="rename-name"
        required
        maxlength={256}
        bind:value={draft}
        disabled={saving}
      /></Field
    >
    {#if error}<Notice variant="destructive" role="alert">{error}</Notice>{/if}
    <Row justify="end"
      ><Button
        variant="outline"
        disabled={saving}
        onclick={() => modal?.close()}>Cancel</Button
      ><Button type="submit" disabled={saving}
        >{saving ? "Saving…" : "Save changes"}</Button
      ></Row
    >
  </Form>
</Modal>
