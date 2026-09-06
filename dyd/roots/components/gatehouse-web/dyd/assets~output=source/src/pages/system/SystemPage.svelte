<script lang="ts">
  import { untrack } from "svelte"
  import { Menu, ShieldCheck } from "@lucide/svelte"
  import type { ActivityClient } from "../../app/activity"
  import type { SystemAccessStatus } from "../../app/access.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import type { Route } from "../../route"
  import { createSystemController } from "./system-controller.svelte"

  let { route, systemAccess, activity, principalID, principalName, onAuthenticationLost, onSystemAccessChange, onLogout, mobileMenuOpen = $bindable() }: {
    route: Route
    systemAccess: SystemAccessStatus
    activity: ActivityClient
    principalID: () => string | undefined
    principalName: string
    onAuthenticationLost: () => void
    onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
    onLogout: () => void
    mobileMenuOpen: boolean
  } = $props()
  const controller = untrack(() => createSystemController({ activity, principalID, onAuthenticationLost, onSystemAccessChange }))

  $effect(() => {
    if (route.kind === "system" || route.kind === "system-grants" || route.kind === "system-principals") {
      void controller.load(route.kind)
    }
  })

  $effect(() => {
    if (systemAccess !== "available") {
      controller.stop()
      return
    }
    return controller.start()
  })
</script>

<div class="app-shell">
  {#if mobileMenuOpen}<button class="mobile-menu-backdrop" type="button" aria-label="Close navigation menu" onclick={() => mobileMenuOpen = false}></button>{/if}
  <aside class:mobile-menu-open={mobileMenuOpen} class="sidebar">
    <RouterLink class="brand" href="/app/">Gatehouse</RouterLink>
    <nav class="sidebar-nav" aria-label="System navigation">
      <section class="sidebar-section">
        <h2>System</h2>
        <ul>
          <li><RouterLink class={route.kind === "system" ? "active" : undefined} href="/app/system">Overview</RouterLink></li>
          {#if systemAccess === "available"}<li><RouterLink class={route.kind === "system-principals" ? "active" : undefined} href="/app/system/principals">Principals</RouterLink></li>{/if}
          {#if systemAccess === "available"}<li><RouterLink class={route.kind === "system-grants" ? "active" : undefined} href="/app/system/grants">System grants</RouterLink></li>{/if}
        </ul>
      </section>
    </nav>
    <div class="sidebar-footer">
      <span>{principalName}</span>
      <button class="button is-small is-danger is-light" type="button" onclick={onLogout}>Log out</button>
    </div>
  </aside>
  <main class="workspace-main">
    <header class="workspace-header system-header">
      <button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}><Menu size={20} strokeWidth={2} aria-hidden="true" /></button>
      <h1 class="workspace-breadcrumb">
        {#if route.kind === "system-grants" || route.kind === "system-principals"}
          <RouterLink class="workspace-breadcrumb-segment" href="/app/system"><ShieldCheck size={18} strokeWidth={2} aria-hidden="true" /><span>System</span></RouterLink>
          <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
          <span>{route.kind === "system-principals" ? "Principals" : "System grants"}</span>
        {:else}
          <span class="workspace-breadcrumb-segment"><ShieldCheck size={18} strokeWidth={2} aria-hidden="true" /><span>System</span></span>
        {/if}
      </h1>
    </header>
    {#if systemAccess === "checking"}
      <section class="system-page"><p class="dashboard-empty">Loading system access...</p></section>
    {:else if systemAccess !== "available"}
      <section class="system-page system-access-denied"><p class="eyebrow">System</p><h2 class="title is-3">System access required</h2><p>You do not currently have an enabled system manager grant.</p></section>
    {:else if route.kind === "system"}
      <section class="system-page">
        <p class="eyebrow">System</p>
        <h2 class="title is-3">System administration</h2>
        <p class="subtitle is-6">Manage global Gatehouse state.</p>
        <RouterLink class="system-section-link" href="/app/system/principals"><span><strong>Principals</strong><small>View and enable or disable principals and their identities.</small></span></RouterLink>
        <RouterLink class="system-section-link" href="/app/system/grants"><span><strong>System grants</strong><small>Grant or revoke system-manager access.</small></span></RouterLink>
      </section>
    {:else if route.kind === "system-principals"}
      <section class="system-page">
        <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Principals</h2><p class="subtitle is-6">Identity associations are shown without credential verifiers.</p></div></div>
        {#if controller.state.principalError !== ""}<p class="help is-danger" aria-live="polite">{controller.state.principalError}</p>{/if}
        <div class="system-principal-list">
          {#each controller.state.principals as principal (principal.id)}
            <article class:system-principal-disabled={!principal.enabled} class="system-principal-row">
              <div><strong>{principal.name ?? principal.alias ?? principal.id}</strong><small>{principal.id}{principal.alias === undefined ? "" : ` / ${principal.alias}`} / revision {principal.revision}</small>{#if principal.identities.length > 0}<div class="system-principal-identities">{#each principal.identities as identity (identity.id)}<span class:has-text-grey={!identity.enabled}>{identity.key} / {identity.id} / revision {identity.revision}{identity.enabled ? "" : " / Disabled"}</span>{/each}</div>{:else}<small>No identities</small>{/if}</div>
              <div class="system-principal-actions"><span class:has-text-success={principal.enabled} class:has-text-grey={!principal.enabled}>{principal.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.updatingPrincipalIDs.has(principal.id)} onclick={() => void controller.setPrincipalEnabled(principal, !principal.enabled)}>{controller.state.updatingPrincipalIDs.has(principal.id) ? "Saving..." : principal.enabled ? "Disable" : "Enable"}</button></div>
            </article>
          {:else}<p class="dashboard-empty">No principals are configured.</p>{/each}
        </div>
      </section>
    {:else}
      <section class="system-page">
        <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">System grants</h2><p class="subtitle is-6">System managers can modify global Gatehouse state.</p></div></div>
        <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void controller.addGrant() }}>
          <label class="field"><span class="label">Principal ID</span><input class="input" autocomplete="off" placeholder="prn_..." bind:value={controller.state.grantPrincipal} /></label>
          <button class="button is-primary" type="submit" disabled={controller.state.creatingGrant}>{controller.state.creatingGrant ? "Granting..." : "Add manager"}</button>
        </form>
        {#if controller.state.grantError !== ""}<p class="help is-danger" aria-live="polite">{controller.state.grantError}</p>{/if}
        <div class="system-grant-list">
          {#each controller.state.grants as grant (grant.ref.id)}
            <article class:system-grant-disabled={!grant.enabled} class="system-grant-row">
              <div><strong>{grant.principal.id}</strong><small>{grant.ref.id} / revision {grant.revision}</small></div>
              <div class="system-grant-actions"><span class:has-text-success={grant.enabled} class:has-text-grey={!grant.enabled}>{grant.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.updatingGrantIDs.has(grant.ref.id)} onclick={() => void controller.setGrantEnabled(grant, !grant.enabled)}>{controller.state.updatingGrantIDs.has(grant.ref.id) ? "Saving..." : grant.enabled ? "Disable" : "Enable"}</button></div>
            </article>
          {:else}<p class="dashboard-empty">No system grants are configured.</p>{/each}
        </div>
      </section>
    {/if}
  </main>
</div>
