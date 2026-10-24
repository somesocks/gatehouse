<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../app/runtime.svelte"
  import SessionChatRoute from "./SessionChatRoute.svelte"
  import SessionFilesRoute from "../session-files/SessionFilesRoute.svelte"
  import SessionNotesRoute from "../session-notes/SessionNotesRoute.svelte"
  import TasksRoute from "../tasks/TasksRoute.svelte"
  import SessionSecretsRoute from "../session-secrets/SessionSecretsRoute.svelte"
  import { createResourcePresentation } from "./resource-presentation.svelte"
  import {
    sessionTool,
    sessionToolDismissPath,
    type ConversationRoute,
    type SessionWorkspaceRoute,
  } from "./session-tools"

  const runtime = useRuntime()
  const presentation = createResourcePresentation()
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
          },
  )
  $effect.pre(() => {
    if (currentRoute.kind === "session-chat") lastConversation = currentRoute
  })
  const tool = $derived(sessionTool(currentRoute))
  const displayedRoute = $derived(presentation.state.route)
  const displayedTool = $derived(displayedRoute ? sessionTool(displayedRoute) : null)
  const canOpen = $derived(
    runtime.auth.state.status === "authenticated" &&
      runtime.access.state.workspaces.some((workspace) => workspace.id === currentRoute.workspaceID),
  )
  $effect.pre(() => {
    const route = currentRoute
    const allowed = canOpen
    const principalID = runtime.auth.state.claims?.principal.ref.id
    untrack(() => presentation.update(route, allowed, principalID))
  })

  function close() {
    if (tool !== null && canOpen)
      runtime.navigate(sessionToolDismissPath(currentRoute, conversation))
  }
  const dialog = $derived({
    open: presentation.state.open,
    onClose: close,
    onClosed: presentation.closed,
  })

</script>

{#key sessionKey}
  <SessionChatRoute route={conversation} />
    {#if displayedRoute}
      {#if displayedTool === "files"}
        <SessionFilesRoute embedded {dialog} route={displayedRoute} suspended={!presentation.state.open} />
      {:else if displayedTool === "notes"}
        <SessionNotesRoute embedded {dialog} route={displayedRoute} suspended={!presentation.state.open} />
      {:else if displayedTool === "tasks"}
        <TasksRoute embedded {dialog} route={displayedRoute} suspended={!presentation.state.open} />
      {:else if displayedTool === "secrets"}
        <SessionSecretsRoute embedded {dialog} route={displayedRoute} suspended={!presentation.state.open} />
      {/if}
    {/if}
{/key}
