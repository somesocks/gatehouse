<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import { createSystemGrantsController } from "./system-grants-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() => createSystemGrantsController({ activity: runtime.activity, onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess, principalID: () => runtime.auth.state.claims?.principal.ref.id }))
  $effect(() => { void controller.load() })
  $effect(() => controller.start())
</script>

<SystemFrame active="grants" title="System grants">
  <section class="system-page">
    <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">System grants</h2><p class="subtitle is-6">System managers can modify global Gatehouse state.</p></div></div>
    <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void controller.create() }}><label class="field"><span class="label">Principal ID</span><input class="input" autocomplete="off" placeholder="prn_..." bind:value={controller.state.principal} /></label><button class="button is-primary" type="submit" disabled={controller.state.creating}>{controller.state.creating ? "Granting..." : "Add manager"}</button></form>
    {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    <div class="system-grant-list">{#each controller.state.grants as grant (grant.ref.id)}<article class:system-grant-disabled={!grant.enabled} class="system-grant-row"><div><strong>{grant.principal.id}</strong><small>{grant.ref.id} / revision {grant.revision}</small></div><div class="system-grant-actions"><span class:has-text-success={grant.enabled} class:has-text-grey={!grant.enabled}>{grant.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.updatingIDs.has(grant.ref.id)} onclick={() => void controller.setEnabled(grant, !grant.enabled)}>{controller.state.updatingIDs.has(grant.ref.id) ? "Saving..." : grant.enabled ? "Disable" : "Enable"}</button></div></article>{:else}<p class="dashboard-empty">No system grants are configured.</p>{/each}</div>
  </section>
</SystemFrame>
