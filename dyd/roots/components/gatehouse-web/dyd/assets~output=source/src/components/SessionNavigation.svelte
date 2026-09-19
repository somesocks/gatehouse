<script lang="ts">
  import { Menu, X } from "@lucide/svelte"
  import { DropdownMenu } from "bits-ui"
  import { updateChatSession } from "../app/chat"
  import { useRuntime } from "../app/runtime.svelte"
  import RouterLink from "./RouterLink.svelte"

  type SessionView = "chat" | "files" | "notes" | "tasks" | "secrets"

  let {
    workspaceID,
    sessionID,
    active,
    name,
    onRenamed,
  }: {
    workspaceID: string
    sessionID: string
    active: SessionView
    name?: string
    onRenamed?: (name: string) => void
  } = $props()
  const runtime = useRuntime()
  let editName = $state("")
  let editError = $state("")
  let updating = $state(false)

  const sessionPath = () =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/ses/${encodeURIComponent(sessionID)}`
  const pathFor = (view: SessionView) =>
    view === "chat" ? sessionPath() : `${sessionPath()}/${view}`

  let editModal = $state<HTMLDialogElement>()
  function openEdit(): void {
    editName = name ?? ""
    editError = ""
    editModal?.showModal()
  }
  function closeEdit(): void {
    if (!updating) editModal?.close()
  }
  async function save(): Promise<void> {
    updating = true
    editError = ""
    try {
      const response = await updateChatSession(workspaceID, sessionID, {
        name: editName,
      })
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("session unavailable")
      const updated = (await response.json()) as { name?: string }
      onRenamed?.(updated.name ?? editName)
      editModal?.close()
    } catch {
      editError = "The chat could not be renamed. Try again."
    } finally {
      updating = false
    }
  }
</script>

<nav aria-label="Session navigation" data-page-navigation>
  <ul>
    <li>
      <button class="small" type="button" onclick={openEdit}>Edit</button>
    </li>
    <li>
      <RouterLink
        aria-current={active === "chat" ? "page" : undefined}
        href={pathFor("chat")}>Chat</RouterLink
      >
    </li>
    <li>
      <RouterLink
        aria-current={active === "files" ? "page" : undefined}
        href={pathFor("files")}>Files</RouterLink
      >
    </li>
    <li>
      <RouterLink
        aria-current={active === "notes" ? "page" : undefined}
        href={pathFor("notes")}>Notes</RouterLink
      >
    </li>
    <li>
      <RouterLink
        aria-current={active === "tasks" ? "page" : undefined}
        href={pathFor("tasks")}>Tasks</RouterLink
      >
    </li>
    <li>
      <RouterLink
        aria-current={active === "secrets" ? "page" : undefined}
        href={pathFor("secrets")}>Secrets</RouterLink
      >
    </li>
  </ul>
  <DropdownMenu.Root>
    <DropdownMenu.Trigger
      class="session-navigation-menu icon inline"
      aria-label="Session navigation"
      title="Session navigation"
      ><Menu size={18} strokeWidth={2} aria-hidden="true" /></DropdownMenu.Trigger
    >
    <DropdownMenu.Portal>
      <DropdownMenu.Content
        class="list dropdown-menu-content"
        sideOffset={6}
        align="end"
      >
        <DropdownMenu.Item class="list-item" onSelect={openEdit}
          >Edit</DropdownMenu.Item
        >
        <DropdownMenu.Item
          class="list-item"
          onSelect={() => runtime.navigate(pathFor("chat"))}>Chat</DropdownMenu.Item
        >
        <DropdownMenu.Item
          class="list-item"
          onSelect={() => runtime.navigate(pathFor("files"))}>Files</DropdownMenu.Item
        >
        <DropdownMenu.Item
          class="list-item"
          onSelect={() => runtime.navigate(pathFor("notes"))}>Notes</DropdownMenu.Item
        >
        <DropdownMenu.Item
          class="list-item"
          onSelect={() => runtime.navigate(pathFor("tasks"))}>Tasks</DropdownMenu.Item
        >
        <DropdownMenu.Item
          class="list-item"
          onSelect={() => runtime.navigate(pathFor("secrets"))}>Secrets</DropdownMenu.Item
        >
      </DropdownMenu.Content>
    </DropdownMenu.Portal>
  </DropdownMenu.Root>
</nav>
<dialog
  bind:this={editModal}
  class="modal"
  oncancel={(event) => {
    if (updating) event.preventDefault()
  }}
>
  <form
    class="modal-body stack"
    onsubmit={(event) => {
      event.preventDefault()
      void save()
    }}
  >
    <header class="modal-header">
      <h2>Rename chat</h2>
      <button class="icon" type="button" aria-label="Close" onclick={closeEdit}>
        <X size={16} />
      </button>
    </header>
    <label class="field">
      Name
      <input
        id="chat-edit-name"
        autocomplete="off"
        maxlength="256"
        required
        bind:value={editName}
      />
    </label>
    {#if editError !== ""}<p class="field-help" role="alert" aria-live="polite">
        {editError}
      </p>{/if}
    <footer class="modal-footer">
      <button type="button" disabled={updating} onclick={closeEdit}>Cancel</button>
      <button class="primary" type="submit" disabled={updating}
        >{updating ? "Saving..." : "Save changes"}</button
      >
    </footer>
  </form>
</dialog>
