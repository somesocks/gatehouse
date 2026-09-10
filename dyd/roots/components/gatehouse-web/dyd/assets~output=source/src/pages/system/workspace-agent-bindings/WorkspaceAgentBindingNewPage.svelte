<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createWorkspaceAgentBindingsController } from "./workspace-agent-bindings-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() => createWorkspaceAgentBindingsController({ activity: runtime.activity, onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))

  async function create(): Promise<void> {
    await controller.create()
    if (controller.state.error === "") runtime.navigate("/app/system/workspace-agent-bindings", true)
  }
</script>

<SystemFrame active="workspace-agent-bindings" title="New workspace agent binding">
  <section class="system-page">
    <div class="system-page-heading mb-5"><div><p class="eyebrow">System</p><h2 class="title is-3">New workspace agent binding</h2></div></div>
    <form onsubmit={(event) => { event.preventDefault(); void create() }}>
      <div class="field"><label class="label" for="agent-binding-workspace">Workspace ID</label><div class="control"><input class="input" id="agent-binding-workspace" required bind:value={controller.state.form.workspace} /></div></div>
      <div class="field"><label class="label" for="agent-binding-alias">Binding alias</label><div class="control"><input class="input" id="agent-binding-alias" required bind:value={controller.state.form.alias} /></div></div>
      <div class="field"><label class="label" for="agent-binding-model">Model ID</label><div class="control"><input class="input" id="agent-binding-model" required bind:value={controller.state.form.model} /></div></div>
      <div class="field"><label class="label" for="agent-binding-priority">Priority</label><div class="control"><input class="input" id="agent-binding-priority" type="number" bind:value={controller.state.form.priority} /></div></div>
      <div class="field"><label class="label" for="agent-binding-label">Label</label><div class="control"><input class="input" id="agent-binding-label" bind:value={controller.state.form.label} /></div></div>
      <div class="field"><label class="label" for="agent-binding-system-prompt">System prompt</label><div class="control"><textarea class="textarea" id="agent-binding-system-prompt" rows="4" bind:value={controller.state.form.systemPrompt}></textarea></div></div>
      <div class="field is-grouped"><p class="control"><button class="button is-primary">Add binding</button></p><p class="control"><RouterLink class="button" href="/app/system/workspace-agent-bindings">Cancel</RouterLink></p></div>
    </form>
    {#if controller.state.error}<p class="help is-danger">{controller.state.error}</p>{/if}
  </section>
</SystemFrame>
