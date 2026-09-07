<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import { createPrincipalsController } from "./principals-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() => createPrincipalsController({ onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess, principalID: () => runtime.auth.state.claims?.principal.ref.id }))
  $effect(() => { void controller.load() })
</script>

<SystemFrame active="principals" title="Principals">
  <section class="system-page">
    <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Principals</h2><p class="subtitle is-6">Identity associations are shown without credential verifiers.</p></div></div>
    {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    <div class="system-principal-list">{#each controller.state.principals as principal (principal.id)}<article class:system-principal-disabled={!principal.enabled} class="system-principal-row"><div><strong>{principal.name ?? principal.alias ?? principal.id}</strong><small>{principal.id}{principal.alias === undefined ? "" : ` / ${principal.alias}`} / revision {principal.revision}</small>{#if principal.identities.length > 0}<div class="system-principal-identities">{#each principal.identities as identity (identity.id)}<span class:has-text-grey={!identity.enabled}>{identity.key} / {identity.id} / revision {identity.revision}{identity.enabled ? "" : " / Disabled"}</span>{/each}</div>{:else}<small>No identities</small>{/if}</div><div class="system-principal-actions"><span class:has-text-success={principal.enabled} class:has-text-grey={!principal.enabled}>{principal.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.updatingIDs.has(principal.id)} onclick={() => void controller.setEnabled(principal, !principal.enabled)}>{controller.state.updatingIDs.has(principal.id) ? "Saving..." : principal.enabled ? "Disable" : "Enable"}</button></div></article>{:else}<p class="dashboard-empty">No principals are configured.</p>{/each}</div>
  </section>
</SystemFrame>
