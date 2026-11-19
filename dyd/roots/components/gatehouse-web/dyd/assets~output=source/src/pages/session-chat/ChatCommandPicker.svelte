<script lang="ts">
  import { tick } from "svelte"
  import * as UI from "../../ui"

  let {
    open = $bindable(false),
    onSendAs,
    onclose,
  }: {
    open?: boolean
    onSendAs: () => void
    onclose: () => void
  } = $props()
  let query = $state("")
  let searchElement = $state<HTMLInputElement>()
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
    <UI.DialogTitle>Chat commands</UI.DialogTitle>
    <UI.Command.Root label="Chat commands">
      <UI.Command.Input
        bind:ref={searchElement}
        bind:value={query}
        placeholder="Search commands"
      />
      <UI.Command.List>
        <UI.Command.Empty>No matching commands.</UI.Command.Empty>
        <UI.Command.Group>
          <UI.Command.Item value="Send as" onSelect={onSendAs}
            >Send as…</UI.Command.Item
          >
        </UI.Command.Group>
      </UI.Command.List>
    </UI.Command.Root>
  </UI.Dialog.Content>
</UI.Dialog.Root>
