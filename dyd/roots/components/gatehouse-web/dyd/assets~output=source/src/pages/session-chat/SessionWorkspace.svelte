<script lang="ts">
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import { routePath } from "../../route"
  import SessionChatRoute from "./SessionChatRoute.svelte"
  import SessionFilesRoute from "../session-files/SessionFilesRoute.svelte"
  import SessionNotesRoute from "../session-notes/SessionNotesRoute.svelte"
  import TasksRoute from "../tasks/TasksRoute.svelte"
  import SessionSecretsRoute from "../session-secrets/SessionSecretsRoute.svelte"
  import {
    sessionTool,
    sessionToolBack,
    type ConversationRoute,
    type SessionWorkspaceRoute,
  } from "./session-tools"

  const runtime = useRuntime()
  const currentRoute = $derived(runtime.state.route as SessionWorkspaceRoute)
  const sessionKey = $derived(`${currentRoute.workspaceID}\u0000${currentRoute.sessionID}`)
  let lastConversation = $state<ConversationRoute | null>(null)
  const conversation = $derived<ConversationRoute>(
    currentRoute.kind === "session-chat"
      ? currentRoute
      : lastConversation?.workspaceID === currentRoute.workspaceID &&
          lastConversation.sessionID === currentRoute.sessionID
        ? lastConversation
        : {
            kind: "session-chat",
            workspaceID: currentRoute.workspaceID,
            sessionID: currentRoute.sessionID,
            mode: "group",
            agent: null,
          },
  )
  $effect.pre(() => {
    if (currentRoute.kind === "session-chat") lastConversation = currentRoute
  })
  const tool = $derived(sessionTool(currentRoute))
  const back = $derived(sessionToolBack(currentRoute))
  const title = $derived(tool === null ? "Chat tools" : tool[0].toUpperCase() + tool.slice(1))
  const canOpen = $derived(
    runtime.auth.state.status === "authenticated" &&
      runtime.access.state.workspaces.some((workspace) => workspace.id === currentRoute.workspaceID),
  )

  function close() {
    if (tool !== null && canOpen) runtime.navigate(routePath(conversation))
  }

  function goBack() {
    if (back !== undefined) runtime.navigate(back.path)
  }
</script>

{#key sessionKey}
  <SessionChatRoute route={conversation} />
  <UI.ResourceDialog
    open={tool !== null && canOpen}
    {title}
    viewKey={routePath(currentRoute)}
    expanded={tool === "notes" && currentRoute.kind !== "session-notes" && currentRoute.kind !== "session-note-history"}
    backLabel={back?.label}
    onBack={back === undefined ? undefined : goBack}
    onClose={close}
  >
    {#if tool === "files"}
      <SessionFilesRoute embedded />
    {:else if tool === "notes"}
      <SessionNotesRoute embedded />
    {:else if tool === "tasks"}
      <TasksRoute embedded />
    {:else if tool === "secrets"}
      <SessionSecretsRoute embedded />
    {/if}
  </UI.ResourceDialog>
{/key}
