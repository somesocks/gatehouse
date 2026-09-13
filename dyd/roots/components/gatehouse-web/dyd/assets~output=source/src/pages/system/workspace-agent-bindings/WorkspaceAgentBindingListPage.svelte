<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createWorkspaceAgentBindingsController } from "./workspace-agent-bindings-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createWorkspaceAgentBindingsController({
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

<SystemFrame active="workspace-agent-bindings" title="Workspace agent bindings"
  ><section class="system-page">
    <div class="system-page-heading mb-5">
      <div>
        <p class="eyebrow">System</p>
        <h2 class="title is-3">Workspace agent bindings</h2>
        <p class="subtitle is-6">Bind global agent models to workspaces.</p>
      </div>
      <RouterLink
        class="button is-primary"
        href="/app/system/workspace-agent-bindings/new">Add binding</RouterLink
      >
    </div>
    <div class="system-grant-list">
      {#each controller.state.bindings as b (b.id)}<RouterLink
          class={`system-grant-row${b.enabled ? "" : " system-grant-disabled"}`}
          href={`/app/system/workspace-agent-bindings/${encodeURIComponent(b.workspace)}/${encodeURIComponent(b.id)}`}
          ><div>
            <strong>{b.workspace} / {b.alias}</strong><small
              >{b.id} / priority {b.priority} / revision {b.revision}</small
            >
          </div>
          <span>{b.enabled ? "Enabled" : "Disabled"}</span></RouterLink
        >{:else}<p class="dashboard-empty">
          No workspace agent bindings are configured.
        </p>{/each}
    </div>
  </section></SystemFrame
>
