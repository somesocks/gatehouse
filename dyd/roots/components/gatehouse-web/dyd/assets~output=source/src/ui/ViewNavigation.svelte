<script lang="ts">
  import * as Menu from "./dropdown-menu"
  import { Menu as MenuIcon } from "@lucide/svelte"
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
  }: {
    items: NavigationItem[]
    name?: string
    onRename?: (name: string) => Promise<void>
  } = $props()
  let modal = $state<DialogHandle>()
  let draft = $state("")
  let error = $state("")
  let saving = $state(false)
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

<Row as="nav" aria-label="View navigation" justify="end" push>
  <Menu.Root>
    <Menu.Trigger aria-label="View navigation"
      ><MenuIcon aria-hidden="true" /></Menu.Trigger
    >
    <Menu.Content align="end">
      {#if onRename}<Menu.Item onSelect={edit}>Rename</Menu.Item>{/if}
      {#each items as item (item.href)}<Menu.Item
          >{#snippet child({ props })}<RouterLink
              {...props}
              href={item.href}
              aria-current={item.active ? "page" : undefined}
              >{item.label}</RouterLink
            >{/snippet}</Menu.Item
        >{/each}
    </Menu.Content>
  </Menu.Root>
</Row>
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
