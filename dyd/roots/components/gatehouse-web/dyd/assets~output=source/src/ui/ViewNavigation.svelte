<script lang="ts">
  import { MediaQuery } from "svelte/reactivity"
  import { Menu as MenuIcon, Check } from "@lucide/svelte"
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
  import type { NavigationItem } from "./navigation"
  import type { DialogHandle } from "./types"
  let {
    items,
    name = "",
    onRename,
    onParticipants,
  }: {
    items: NavigationItem[]
    name?: string
    onRename?: (name: string) => Promise<void>
    onParticipants?: () => void
  } = $props()
  let modal = $state<DialogHandle>()
  let draft = $state("")
  let error = $state("")
  let saving = $state(false)
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

<nav class="ml-auto flex min-w-0 items-center gap-1" aria-label="Chat views">
  {#if mobile.current}
    <Menu.Root>
      <Menu.Trigger aria-label="Chat views" title="Chat views"
        ><MenuIcon aria-hidden="true" /></Menu.Trigger
      >
      <Menu.Content align="end">
        {#each items as item (item.href)}<Menu.Item>
            {#snippet child({ props })}<RouterLink
                {...props}
                href={item.href}
                aria-current={item.active ? "page" : undefined}
              >
                {item.label}{#if item.active}<Check
                    size={16}
                    aria-hidden="true"
                  />{/if}
              </RouterLink>{/snippet}
          </Menu.Item>{/each}
        {#if onParticipants}<Menu.Separator /><Menu.Item
            onSelect={onParticipants}>Participants</Menu.Item
          >{/if}
        {#if onRename}<Menu.Separator /><Menu.Item onSelect={edit}
            >Rename</Menu.Item
          >{/if}
      </Menu.Content>
    </Menu.Root>
  {:else}
    {#each items as item (item.href)}<RouterLink
        variant={item.active ? "outline" : "ghost"}
        size="sm"
        href={item.href}
        aria-current={item.active ? "page" : undefined}>{item.label}</RouterLink
      >{/each}
    {#if onParticipants}<Button
        variant="ghost"
        size="sm"
        onclick={onParticipants}>Participants</Button
      >{/if}
    {#if onRename}<Button variant="ghost" size="sm" onclick={edit}
        >Rename</Button
      >{/if}
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
