<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import { createWorkspaceStorageBindingsController } from "./workspace-storage-bindings-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() => createWorkspaceStorageBindingsController({ activity: runtime.activity, onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))

  $effect(() => { void controller.load() })
  $effect(() => controller.start())
</script>

<SystemFrame active="workspace-storage-bindings" title="Workspace storage bindings">
  <section class="system-page">
    <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Workspace storage bindings</h2><p class="subtitle is-6">Bind global storage providers to workspaces. Disabling preserves the binding.</p></div></div>
    <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void controller.create() }}><label class="field"><span class="label">Workspace ID</span><input class="input" required bind:value={controller.state.form.workspace} /></label><label class="field"><span class="label">Storage provider ID</span><input class="input" required bind:value={controller.state.form.provider} /></label><label class="field"><span class="label">Priority</span><input class="input" type="number" bind:value={controller.state.form.priority} /></label><button class="button is-primary" type="submit" disabled={controller.state.saving}>Bind storage provider</button></form>
    {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    <div class="system-grant-list">{#each controller.state.bindings as binding (`${binding.workspace}/${binding.provider}`)}<article class:system-grant-disabled={!binding.enabled} class="system-grant-row"><div><strong>{binding.workspace} / {binding.provider}</strong><small>priority {binding.priority} / revision {binding.revision}</small></div><div class="system-grant-actions"><span>{binding.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.saving} onclick={() => void controller.setEnabled(binding, !binding.enabled)}>{binding.enabled ? "Disable" : "Enable"}</button></div></article>{:else}<p class="dashboard-empty">No workspace storage bindings are configured.</p>{/each}</div>
  </section>
</SystemFrame>
