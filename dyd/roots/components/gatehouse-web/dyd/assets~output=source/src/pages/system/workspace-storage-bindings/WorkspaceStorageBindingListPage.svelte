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
  $effect(() => {
    void controller.load()
  })
  $effect(() => controller.start())
</script>

<SystemFrame
  active="workspace-storage-bindings"
  title="Workspace storage bindings"
  ><section class="stack">
    <div class="split">
      <div>
        <p class="eyebrow">System</p>
        <h2>Workspace storage bindings</h2>
        <p>
          Bind global storage providers to workspaces.
        </p>
      </div>
      <RouterLink
        class="primary"
        href="/app/system/workspace-storage-bindings/new"
        >Add binding</RouterLink
      >
    </div>
    <div class="list">
      {#each controller.state.bindings as b (`${b.workspace}/${b.provider}`)}<RouterLink
          class="list-item surface split"
          data-disabled={!b.enabled || undefined}
          href={`/app/system/workspace-storage-bindings/${encodeURIComponent(b.workspace)}/${encodeURIComponent(b.provider)}`}
          ><div class="stack">
            <strong>{b.workspace} / {b.provider}</strong><small
              >priority {b.priority} / revision {b.revision}</small
            >
          </div>
          <span>{b.enabled ? "Enabled" : "Disabled"}</span></RouterLink
        >{:else}<p class="muted">
          No workspace storage bindings are configured.
        </p>{/each}
    </div>
  </section></SystemFrame
>
