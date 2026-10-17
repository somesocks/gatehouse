<script lang="ts">
  import { untrack } from "svelte"
  import * as UI from "../ui"
  import type { NavigationItem } from "../ui/navigation"
  import { useRuntime } from "../app/runtime.svelte"
  import { createSessionParticipantsController } from "./session-participants-controller.svelte"
  import SessionParticipantsDialog from "./SessionParticipantsDialog.svelte"
  let {
    workspaceID,
    sessionID,
    items,
    name = "",
    onRename,
  }: {
    workspaceID: string
    sessionID: string
    items: NavigationItem[]
    name?: string
    onRename?: (name: string) => Promise<void>
  } = $props()
  const runtime = useRuntime()
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
  {items}
  {name}
  {onRename}
  onParticipants={() => {
    modal?.showModal()
    void controller.open()
  }}
/>
<SessionParticipantsDialog {workspaceID} {sessionID} {controller} bind:modal />
