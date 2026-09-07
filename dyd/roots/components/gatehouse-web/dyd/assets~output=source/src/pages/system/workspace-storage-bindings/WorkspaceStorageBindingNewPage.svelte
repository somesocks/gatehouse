<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createWorkspaceStorageBindingsController } from "./workspace-storage-bindings-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() => createWorkspaceStorageBindingsController({ activity: runtime.activity, onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))

  async function create(): Promise<void> {
    await controller.create()
    if (!controller.state.error) runtime.navigate("/app/system/workspace-storage-bindings", true)
  }
</script>

<SystemFrame active="workspace-storage-bindings" title="New workspace storage binding">
  <section class="system-page">
    <div class="system-page-heading mb-5"><div><p class="eyebrow">System</p><h2 class="title is-3">New workspace storage binding</h2></div></div>
    <form onsubmit={(event) => { event.preventDefault(); void create() }}>
      <div class="field"><label class="label" for="storage-binding-workspace">Workspace ID</label><div class="control"><input class="input" id="storage-binding-workspace" required bind:value={controller.state.form.workspace} /></div></div>
      <div class="field"><label class="label" for="storage-binding-provider">Storage provider ID</label><div class="control"><input class="input" id="storage-binding-provider" required bind:value={controller.state.form.provider} /></div></div>
      <div class="field"><label class="label" for="storage-binding-priority">Priority</label><div class="control"><input class="input" id="storage-binding-priority" type="number" bind:value={controller.state.form.priority} /></div></div>
      <div class="field is-grouped"><p class="control"><button class="button is-primary">Add binding</button></p><p class="control"><RouterLink class="button" href="/app/system/workspace-storage-bindings">Cancel</RouterLink></p></div>
    </form>
    {#if controller.state.error}<p class="help is-danger">{controller.state.error}</p>{/if}
  </section>
</SystemFrame>
