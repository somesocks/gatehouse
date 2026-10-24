<script lang="ts">
  import { untrack } from "svelte"
  import * as UI from "../../ui"
  import type { ActivityClient } from "../../app/activity"
  import SessionParticipantsDialog from "../../session-participants/SessionParticipantsDialog.svelte"
  import { createSessionParticipantsController } from "../../session-participants/session-participants-controller.svelte"

  const controller = createSessionParticipantsController({
    activity: { subscribe: () => () => {}, poll: async () => {} } as unknown as ActivityClient,
    onAuthenticationLost() {},
    onAccessLost() {},
  })
  let modal = $state<UI.DialogHandle>()
  $effect(() => untrack(() => controller.start("workspace", "chat")))
</script>

<UI.Button disabled={!modal} onclick={() => { modal?.showModal(); void controller.open() }}>Open participants</UI.Button>
<SessionParticipantsDialog workspaceID="workspace" sessionID="chat" {controller} bind:modal />
