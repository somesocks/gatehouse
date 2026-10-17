<script lang="ts">
  import { untrack } from "svelte"
  import * as UI from "../ui"
  import type { SessionNavigationItem } from "../ui/navigation"
  import { useRuntime } from "../app/runtime.svelte"
  import { createSessionParticipantsController } from "./session-participants-controller.svelte"
  import SessionParticipantsDialog from "./SessionParticipantsDialog.svelte"
  import { sessionTool } from "../pages/session-chat/session-tools"
  let {
    workspaceID,
    sessionID,
    items,
    name = "",
    onRename,
  }: {
    workspaceID: string
    sessionID: string
    items: SessionNavigationItem[]
    name?: string
    onRename?: (name: string) => Promise<void>
  } = $props()
  const runtime = useRuntime()
  const toolItems = $derived(
    items.filter((item) => item.icon !== undefined).map((item) => ({
      ...item,
      active: "sessionID" in runtime.state.route &&
        item.icon === sessionTool(runtime.state.route),
    })),
  )
  let modal = $state<UI.DialogHandle>()
  const controller = createSessionParticipantsController({
    activity: runtime.activity,
    onAuthenticationLost: runtime.requireLogin,
    onAccessLost: () => {
      modal?.close()
      runtime.navigate(`/app/wsp/${encodeURIComponent(workspaceID)}/ses`, true)
    },
  })
  $effect(() => {
    const workspace = workspaceID,
      session = sessionID
    runtime.auth.state.claims?.principal.ref.id
    return untrack(() => controller.start(workspace, session))
  })
</script>

<UI.ViewNavigation
  items={toolItems}
  {name}
  {onRename}
  participantsOpen={controller.state.open}
  onParticipants={() => {
    modal?.showModal()
    void controller.open()
  }}
/>
<SessionParticipantsDialog {workspaceID} {sessionID} {controller} bind:modal />
