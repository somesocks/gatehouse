<script lang="ts">
  import { Menu, ShieldCheck } from "@lucide/svelte"
  import type { SystemAccessStatus } from "../../app/access.svelte"
  import RouterLink from "../../components/RouterLink.svelte"

  let { systemAccess, principalName, onLogout, mobileMenuOpen = $bindable() }: { systemAccess: SystemAccessStatus; principalName: string; onLogout: () => void; mobileMenuOpen: boolean } = $props()
</script>

<div class="app-shell">
  {#if mobileMenuOpen}<button class="mobile-menu-backdrop" type="button" aria-label="Close navigation menu" onclick={() => mobileMenuOpen = false}></button>{/if}
  <aside class:mobile-menu-open={mobileMenuOpen} class="sidebar">
    <RouterLink class="brand" href="/app/">Gatehouse</RouterLink>
    <nav class="sidebar-nav" aria-label="System navigation"><section class="sidebar-section"><h2>System</h2><ul>
      <li><RouterLink class="active" href="/app/system">Overview</RouterLink></li>
      {#if systemAccess === "available"}<li><RouterLink href="/app/system/principals">Principals</RouterLink></li>{/if}
      {#if systemAccess === "available"}<li><RouterLink href="/app/system/grants">System grants</RouterLink></li>{/if}
      {#if systemAccess === "available"}<li><RouterLink href="/app/system/agent-providers">Agent providers</RouterLink></li>{/if}
      {#if systemAccess === "available"}<li><RouterLink href="/app/system/agent-models">Agent models</RouterLink></li>{/if}
      {#if systemAccess === "available"}<li><RouterLink href="/app/system/storage-providers">Storage providers</RouterLink></li>{/if}
      {#if systemAccess === "available"}<li><RouterLink href="/app/system/workspace-agent-bindings">Workspace agent bindings</RouterLink></li>{/if}
      {#if systemAccess === "available"}<li><RouterLink href="/app/system/workspace-storage-bindings">Workspace storage bindings</RouterLink></li>{/if}
    </ul></section></nav>
    <div class="sidebar-footer"><span>{principalName}</span><button class="button is-small is-danger is-light" type="button" onclick={onLogout}>Log out</button></div>
  </aside>
  <main class="workspace-main">
    <header class="workspace-header system-header"><button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}><Menu size={20} strokeWidth={2} aria-hidden="true" /></button><h1 class="workspace-breadcrumb"><span class="workspace-breadcrumb-segment"><ShieldCheck size={18} strokeWidth={2} aria-hidden="true" /><span>System</span></span></h1></header>
    {#if systemAccess === "checking"}
      <section class="system-page"><p class="dashboard-empty">Loading system access...</p></section>
    {:else if systemAccess !== "available"}
      <section class="system-page system-access-denied"><p class="eyebrow">System</p><h2 class="title is-3">System access required</h2><p>You do not currently have an enabled system manager grant.</p></section>
    {:else}
      <section class="system-page"><p class="eyebrow">System</p><h2 class="title is-3">System administration</h2><p class="subtitle is-6">Manage global Gatehouse state.</p><RouterLink class="system-section-link" href="/app/system/principals"><span><strong>Principals</strong><small>View and enable or disable principals and their identities.</small></span></RouterLink><RouterLink class="system-section-link" href="/app/system/grants"><span><strong>System grants</strong><small>Grant or revoke system-manager access.</small></span></RouterLink><RouterLink class="system-section-link" href="/app/system/agent-providers"><span><strong>Agent providers</strong><small>Configure model-provider protocols and credentials.</small></span></RouterLink><RouterLink class="system-section-link" href="/app/system/agent-models"><span><strong>Agent models</strong><small>Configure models available for workspace bindings.</small></span></RouterLink><RouterLink class="system-section-link" href="/app/system/storage-providers"><span><strong>Storage providers</strong><small>Configure embedded or S3 storage.</small></span></RouterLink><RouterLink class="system-section-link" href="/app/system/workspace-agent-bindings"><span><strong>Workspace agent bindings</strong><small>Assign agent models to workspaces.</small></span></RouterLink><RouterLink class="system-section-link" href="/app/system/workspace-storage-bindings"><span><strong>Workspace storage bindings</strong><small>Assign storage providers to workspaces.</small></span></RouterLink></section>
    {/if}
  </main>
</div>
