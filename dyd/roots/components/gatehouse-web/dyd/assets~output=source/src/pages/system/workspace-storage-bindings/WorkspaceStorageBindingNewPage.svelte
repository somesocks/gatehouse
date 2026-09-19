<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createWorkspaceStorageBindingsController } from "./workspace-storage-bindings-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createWorkspaceStorageBindingsController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )

  async function create(): Promise<void> {
    await controller.create()
    if (!controller.state.error)
      runtime.navigate("/app/system/workspace-storage-bindings", true)
  }
</script>

<SystemFrame
  active="workspace-storage-bindings"
  title="New workspace storage binding"
>
  <section class="stack">
    <div class="stack">
      <div>
        <p class="eyebrow">System</p>
        <h2>New workspace storage binding</h2>
      </div>
    </div>
    <form
      class="stack"
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <div class="field">
        <label for="storage-binding-workspace">Workspace ID</label
        >
        <div>
          <input
            id="storage-binding-workspace"
            required
            bind:value={controller.state.form.workspace}
          />
        </div>
      </div>
      <div class="field">
        <label for="storage-binding-provider"
          >Storage provider ID</label
        >
        <div>
          <input
            id="storage-binding-provider"
            required
            bind:value={controller.state.form.provider}
          />
        </div>
      </div>
      <div class="field">
        <label for="storage-binding-priority">Priority</label>
        <div>
          <input
            id="storage-binding-priority"
            type="number"
            bind:value={controller.state.form.priority}
          />
        </div>
      </div>
      <div class="cluster">
        <div>
          <button class="primary">Add binding</button>
        </div>
        <div>
          <RouterLink
            class="secondary"
            href="/app/system/workspace-storage-bindings">Cancel</RouterLink
          >
        </div>
      </div>
    </form>
    {#if controller.state.error}<p class="field-help" role="alert">
        {controller.state.error}
      </p>{/if}
  </section>
</SystemFrame>
