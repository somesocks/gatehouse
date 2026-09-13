<script lang="ts">
  import { useRuntime } from "../app/runtime.svelte"
  import RouterLink from "./RouterLink.svelte"

  type SessionView = "chat" | "notes" | "tasks" | "secrets"

  let {
    workspaceID,
    sessionID,
    active,
  }: { workspaceID: string; sessionID: string; active: SessionView } = $props()
  const runtime = useRuntime()

  const sessionPath = () =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/ses/${encodeURIComponent(sessionID)}`
  const pathFor = (view: SessionView) =>
    view === "chat" ? sessionPath() : `${sessionPath()}/${view}`

  function navigate(event: Event): void {
    if (!(event.currentTarget instanceof HTMLSelectElement)) return
    runtime.navigate(pathFor(event.currentTarget.value as SessionView))
  }
</script>

<nav class="session-tabs" aria-label="Session navigation">
  <RouterLink
    class={active === "chat" ? "active" : undefined}
    href={pathFor("chat")}>Chat</RouterLink
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
  <option value="notes">Notes</option>
  <option value="tasks">Tasks</option>
  <option value="secrets">Secrets</option>
</select>
