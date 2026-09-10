<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createWorkspaceAgentBindingController } from "./workspace-agent-binding-controller.svelte"

  let { workspaceID, bindingID }: { workspaceID: string; bindingID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() => createWorkspaceAgentBindingController({ onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))
  $effect(() => { void controller.load(workspaceID, bindingID) })
</script>

<SystemFrame active="workspace-agent-bindings" title="Workspace agent binding">
  <section class="system-page">
    {#if controller.state.loading}
      <p class="dashboard-empty">Loading binding...</p>
    {:else if controller.state.binding === null}
      <p class="help is-danger">{controller.state.error || "Workspace agent binding was not found."}</p>
      <RouterLink class="button" href="/app/system/workspace-agent-bindings">Back to bindings</RouterLink>
    {:else if !controller.state.editing}
      <div class="system-page-heading mb-5"><div><p class="eyebrow">Workspace agent binding</p><h2 class="title is-3">{controller.state.binding.workspace} / {controller.state.binding.alias}</h2></div><button class="button is-primary" type="button" onclick={() => controller.edit()}>Edit</button></div>
      <dl><div class="field"><dt class="label">Priority</dt><dd>{controller.state.binding.priority}</dd></div><div class="field"><dt class="label">Label</dt><dd>{controller.state.binding.label ?? "Not configured"}</dd></div><div class="field"><dt class="label">System prompt</dt><dd>{controller.state.binding.system_prompt ?? "Not configured"}</dd></div><div class="field"><dt class="label">Status</dt><dd>{controller.state.binding.enabled ? "Enabled" : "Disabled"}</dd></div></dl>
    {:else}
      <div class="system-page-heading mb-5"><div><p class="eyebrow">Workspace agent binding</p><h2 class="title is-3">Edit binding</h2></div></div>
      <form onsubmit={(event) => { event.preventDefault(); void controller.update() }}>
        <div class="field"><label class="label" for="agent-binding-workspace">Workspace ID</label><div class="control"><input class="input" id="agent-binding-workspace" disabled value={controller.state.form.workspace} /></div></div>
        <div class="field"><label class="label" for="agent-binding-alias">Alias</label><div class="control"><input class="input" id="agent-binding-alias" disabled value={controller.state.form.alias} /></div></div>
        <div class="field"><label class="label" for="agent-binding-model">Model ID</label><div class="control"><input class="input" id="agent-binding-model" bind:value={controller.state.form.model} /></div></div>
        <div class="field"><label class="label" for="agent-binding-priority">Priority</label><div class="control"><input class="input" id="agent-binding-priority" type="number" bind:value={controller.state.form.priority} /></div></div>
        <div class="field"><label class="label" for="agent-binding-label">Label</label><div class="control"><input class="input" id="agent-binding-label" bind:value={controller.state.form.label} /></div></div>
        <div class="field"><label class="label" for="agent-binding-system-prompt">System prompt</label><div class="control"><textarea class="textarea" id="agent-binding-system-prompt" rows="4" bind:value={controller.state.form.systemPrompt}></textarea></div></div>
        <div class="field"><label class="checkbox"><input type="checkbox" bind:checked={controller.state.form.enabled} /> Enabled</label></div>
        <div class="field is-grouped"><p class="control"><button class="button is-primary">Save changes</button></p><p class="control"><button class="button" type="button" onclick={() => controller.state.editing = false}>Cancel</button></p></div>
      </form>
      {#if controller.state.error}<p class="help is-danger">{controller.state.error}</p>{/if}
    {/if}
  </section>
</SystemFrame>
