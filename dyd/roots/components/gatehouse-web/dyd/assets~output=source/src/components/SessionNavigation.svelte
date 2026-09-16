<script lang="ts">
  import { Button, Dialog } from "bits-ui"
  import { X } from "@lucide/svelte"
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
  let editOpen = $state(false)
  let editName = $state("")
  let editError = $state("")
  let updating = $state(false)

  const sessionPath = () =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/ses/${encodeURIComponent(sessionID)}`
  const pathFor = (view: SessionView) =>
    view === "chat" ? sessionPath() : `${sessionPath()}/${view}`

  function navigate(event: Event): void {
    if (!(event.currentTarget instanceof HTMLSelectElement)) return
    runtime.navigate(pathFor(event.currentTarget.value as SessionView))
  }
  function openEdit(): void {
    editName = name ?? ""
    editError = ""
    editOpen = true
  }
  function closeEdit(): void {
    if (!updating) editOpen = false
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
      editOpen = false
    } catch {
      editError = "The chat could not be renamed. Try again."
    } finally {
      updating = false
    }
  }
</script>

<nav class="session-tabs" aria-label="Session navigation">
  <button type="button" onclick={openEdit}>Edit</button>
  <RouterLink
    class={active === "chat" ? "active" : undefined}
    href={pathFor("chat")}>Chat</RouterLink
  >
  <RouterLink
    class={active === "files" ? "active" : undefined}
    href={pathFor("files")}>Files</RouterLink
  >
  <RouterLink
    class={active === "notes" ? "active" : undefined}
    href={pathFor("notes")}>Notes</RouterLink
  >
  <RouterLink
    class={active === "tasks" ? "active" : undefined}
    href={pathFor("tasks")}>Tasks</RouterLink
  >
  <RouterLink
    class={active === "secrets" ? "active" : undefined}
    href={pathFor("secrets")}>Secrets</RouterLink
  >
</nav>
<select
  class="session-tabs-select"
  aria-label="Session view"
  value={active}
  onchange={navigate}
>
  <option value="chat">Chat</option>
  <option value="files">Files</option>
  <option value="notes">Notes</option>
  <option value="tasks">Tasks</option>
  <option value="secrets">Secrets</option>
</select>
<Dialog.Root bind:open={editOpen}
  ><Dialog.Portal
    ><Dialog.Overlay class="brand-dialog-overlay" /><Dialog.Content
      class="brand-dialog-content"
      ><form
        class="brand-dialog-form"
        onsubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <div class="brand-dialog-heading">
          <Dialog.Title class="brand-dialog-title">Rename chat</Dialog.Title
          ><Button.Root
            class="brand-icon-button"
            type="button"
            aria-label="Close"
            onclick={closeEdit}
            ><X size={18} strokeWidth={2} aria-hidden="true" /></Button.Root
          >
        </div>
        <div class="brand-field">
          <label for="chat-edit-name">Name</label><input
            class="brand-input"
            id="chat-edit-name"
            autocomplete="off"
            maxlength="256"
            required
            bind:value={editName}
          />
        </div>
        {#if editError !== ""}<p class="brand-form-error" aria-live="polite">
            {editError}
          </p>{/if}
        <div class="brand-dialog-actions">
          <Button.Root
            class="brand-button"
            type="button"
            disabled={updating}
            onclick={closeEdit}>Cancel</Button.Root
          ><Button.Root
            class="brand-button brand-button--primary"
            type="submit"
            disabled={updating}
            >{updating ? "Saving..." : "Save changes"}</Button.Root
          >
        </div>
      </form></Dialog.Content
    ></Dialog.Portal
  ></Dialog.Root
>
