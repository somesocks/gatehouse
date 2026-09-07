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
  const agentModel = $state({ alias: "", provider: "", model: "", parameters: "", compaction: "", maxTurns: 0, maxOutputTokens: 0 })
  const storageProvider = $state({ alias: "", protocol: "embedded", endpoint: "", region: "", bucket: "", accessKeyID: "", keychain: "", secretAccessKey: "" })
  const workspaceAgent = $state({ workspace: "", model: "", priority: 0, label: "", systemPrompt: "" })
  const workspaceStorage = $state({ workspace: "", provider: "", priority: 0 })
  const optional = (value: string): string | undefined => value.trim() === "" ? undefined : value.trim()
  const reset = (form: Record<string, string | number>) => { for (const key of Object.keys(form)) form[key] = typeof form[key] === "number" ? 0 : "" }
  async function createAgentModel() { if (await controller.saveAdministration("agent-models", "POST", { alias: agentModel.alias, provider: agentModel.provider, model: agentModel.model, parameters: agentModel.parameters, compaction: agentModel.compaction, max_turns: agentModel.maxTurns, max_output_tokens: agentModel.maxOutputTokens, enabled: true }, () => controller.loadAdministration("agent-models", "agentModels"))) reset(agentModel) }
  async function createStorageProvider() { if (await controller.saveAdministration("storage-providers", "POST", { alias: storageProvider.alias, protocol: storageProvider.protocol, endpoint: optional(storageProvider.endpoint), region: optional(storageProvider.region), bucket: optional(storageProvider.bucket), access_key_id: optional(storageProvider.accessKeyID), keychain: optional(storageProvider.keychain), secret_access_key: optional(storageProvider.secretAccessKey), enabled: true }, () => controller.loadAdministration("storage-providers", "storageProviders"))) reset(storageProvider) }
  async function createWorkspaceAgent() { if (await controller.saveAdministration(`workspace-agents/${encodeURIComponent(workspaceAgent.workspace)}/${encodeURIComponent(workspaceAgent.model)}`, "POST", { priority: workspaceAgent.priority, label: optional(workspaceAgent.label), system_prompt: optional(workspaceAgent.systemPrompt), enabled: true }, () => controller.loadAdministration("workspace-agents", "workspaceAgents"))) reset(workspaceAgent) }
  async function createWorkspaceStorage() { if (await controller.saveAdministration(`workspace-storage-providers/${encodeURIComponent(workspaceStorage.workspace)}/${encodeURIComponent(workspaceStorage.provider)}`, "POST", { priority: workspaceStorage.priority, enabled: true }, () => controller.loadAdministration("workspace-storage-providers", "workspaceStorageProviders"))) reset(workspaceStorage) }

  $effect(() => {
    if (route.kind === "system" || route.kind === "system-grants" || route.kind === "system-principals" || route.kind === "system-agent-models" || route.kind === "system-storage-providers" || route.kind === "system-workspace-bindings") {
      void controller.load(route.kind)
    }
  })

  $effect(() => {
    if (systemAccess !== "available") {
      controller.stop()
      return
    }
    return controller.start(route.kind)
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
           {#if systemAccess === "available"}<li><RouterLink class={route.kind === "system-agent-providers" ? "active" : undefined} href="/app/system/agent-providers">Agent providers</RouterLink></li>{/if}
           {#if systemAccess === "available"}<li><RouterLink class={route.kind === "system-agent-models" ? "active" : undefined} href="/app/system/agent-models">Agent models</RouterLink></li>{/if}
           {#if systemAccess === "available"}<li><RouterLink class={route.kind === "system-storage-providers" ? "active" : undefined} href="/app/system/storage-providers">Storage providers</RouterLink></li>{/if}
           {#if systemAccess === "available"}<li><RouterLink class={route.kind === "system-workspace-bindings" ? "active" : undefined} href="/app/system/workspace-bindings">Workspace bindings</RouterLink></li>{/if}
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
        {#if route.kind !== "system"}
          <RouterLink class="workspace-breadcrumb-segment" href="/app/system"><ShieldCheck size={18} strokeWidth={2} aria-hidden="true" /><span>System</span></RouterLink>
          <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
          <span>{route.kind === "system-principals" ? "Principals" : route.kind === "system-grants" ? "System grants" : route.kind === "system-agent-models" ? "Agent models" : route.kind === "system-storage-providers" ? "Storage providers" : "Workspace bindings"}</span>
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
         <RouterLink class="system-section-link" href="/app/system/agent-providers"><span><strong>Agent providers</strong><small>Configure model-provider protocols and credentials.</small></span></RouterLink>
         <RouterLink class="system-section-link" href="/app/system/agent-models"><span><strong>Agent models</strong><small>Configure models available for workspace bindings.</small></span></RouterLink>
         <RouterLink class="system-section-link" href="/app/system/storage-providers"><span><strong>Storage providers</strong><small>Configure embedded or S3 storage.</small></span></RouterLink>
         <RouterLink class="system-section-link" href="/app/system/workspace-bindings"><span><strong>Workspace bindings</strong><small>Assign agent models and storage providers to workspaces.</small></span></RouterLink>
      </section>
    {:else if route.kind === "system-agent-models"}
      <section class="system-page">
        <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Agent models</h2><p class="subtitle is-6">Models with the same workspace priority are selected randomly.</p></div></div>
        <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void createAgentModel() }}>
          <label class="field"><span class="label">Alias</span><input class="input" required bind:value={agentModel.alias} /></label><label class="field"><span class="label">Provider ID</span><input class="input" required bind:value={agentModel.provider} /></label><label class="field"><span class="label">Model</span><input class="input" required bind:value={agentModel.model} /></label><label class="field"><span class="label">Parameters</span><input class="input" bind:value={agentModel.parameters} /></label><label class="field"><span class="label">Compaction</span><input class="input" bind:value={agentModel.compaction} /></label><label class="field"><span class="label">Max turns</span><input class="input" type="number" min="0" bind:value={agentModel.maxTurns} /></label><label class="field"><span class="label">Max output tokens</span><input class="input" type="number" min="0" bind:value={agentModel.maxOutputTokens} /></label><button class="button is-primary" type="submit" disabled={controller.state.savingAdministration}>Add model</button>
        </form>
        {#if controller.state.administrationError !== ""}<p class="help is-danger" aria-live="polite">{controller.state.administrationError}</p>{/if}
        <div class="system-grant-list">{#each controller.state.agentModels as model (model.id)}<article class:system-grant-disabled={!model.enabled} class="system-grant-row"><div><strong>{model.alias}</strong><small>{model.id} / {model.provider} / {model.model} / revision {model.revision}</small></div><div class="system-grant-actions"><span>{model.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.savingAdministration} onclick={() => void controller.saveAdministration(`agent-models/${encodeURIComponent(model.id)}`, "PATCH", { alias: model.alias, provider: model.provider, model: model.model, parameters: model.parameters, compaction: model.compaction, max_turns: model.max_turns, max_output_tokens: model.max_output_tokens, enabled: !model.enabled, expected_revision: model.revision }, () => controller.loadAdministration("agent-models", "agentModels"))}>{model.enabled ? "Disable" : "Enable"}</button></div></article>{:else}<p class="dashboard-empty">No agent models are configured.</p>{/each}</div>
      </section>
    {:else if route.kind === "system-storage-providers"}
      <section class="system-page">
        <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Storage providers</h2><p class="subtitle is-6">Credentials are write-only. Embedded storage needs no credentials.</p></div></div>
        <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void createStorageProvider() }}>
          <label class="field"><span class="label">Alias</span><input class="input" required bind:value={storageProvider.alias} /></label><label class="field"><span class="label">Protocol</span><select class="select" bind:value={storageProvider.protocol}><option value="embedded">Embedded</option><option value="s3">S3</option></select></label><label class="field"><span class="label">Endpoint</span><input class="input" bind:value={storageProvider.endpoint} /></label><label class="field"><span class="label">Region</span><input class="input" bind:value={storageProvider.region} /></label><label class="field"><span class="label">Bucket</span><input class="input" bind:value={storageProvider.bucket} /></label><label class="field"><span class="label">Access key ID</span><input class="input" bind:value={storageProvider.accessKeyID} /></label><label class="field"><span class="label">Keychain ID</span><input class="input" bind:value={storageProvider.keychain} /></label><label class="field"><span class="label">Secret access key</span><input class="input" type="password" autocomplete="new-password" bind:value={storageProvider.secretAccessKey} /></label><button class="button is-primary" type="submit" disabled={controller.state.savingAdministration}>Add provider</button>
        </form>
        {#if controller.state.administrationError !== ""}<p class="help is-danger" aria-live="polite">{controller.state.administrationError}</p>{/if}
        <div class="system-grant-list">{#each controller.state.storageProviders as provider (provider.id)}<article class:system-grant-disabled={!provider.enabled} class="system-grant-row"><div><strong>{provider.alias}</strong><small>{provider.id} / {provider.protocol} / {provider.bucket ?? "no bucket"} / revision {provider.revision}{provider.credential_configured ? " / credential configured" : ""}</small></div><div class="system-grant-actions"><span>{provider.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.savingAdministration} onclick={() => void controller.saveAdministration(`storage-providers/${encodeURIComponent(provider.id)}`, "PATCH", { alias: provider.alias, protocol: provider.protocol, endpoint: provider.endpoint, region: provider.region, bucket: provider.bucket, access_key_id: provider.access_key_id, keychain: provider.keychain?.id, enabled: !provider.enabled, expected_revision: provider.revision }, () => controller.loadAdministration("storage-providers", "storageProviders"))}>{provider.enabled ? "Disable" : "Enable"}</button></div></article>{:else}<p class="dashboard-empty">No storage providers are configured.</p>{/each}</div>
      </section>
    {:else if route.kind === "system-workspace-bindings"}
      <section class="system-page">
        <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Workspace bindings</h2><p class="subtitle is-6">Bind global models and storage providers to a workspace. Disabling preserves the binding.</p></div></div>
        <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void createWorkspaceAgent() }}><label class="field"><span class="label">Workspace ID</span><input class="input" required bind:value={workspaceAgent.workspace} /></label><label class="field"><span class="label">Model ID</span><input class="input" required bind:value={workspaceAgent.model} /></label><label class="field"><span class="label">Priority</span><input class="input" type="number" bind:value={workspaceAgent.priority} /></label><label class="field"><span class="label">Label</span><input class="input" bind:value={workspaceAgent.label} /></label><label class="field"><span class="label">System prompt</span><textarea class="textarea" rows="2" bind:value={workspaceAgent.systemPrompt}></textarea></label><button class="button is-primary" type="submit" disabled={controller.state.savingAdministration}>Bind agent model</button></form>
        <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void createWorkspaceStorage() }}><label class="field"><span class="label">Workspace ID</span><input class="input" required bind:value={workspaceStorage.workspace} /></label><label class="field"><span class="label">Storage provider ID</span><input class="input" required bind:value={workspaceStorage.provider} /></label><label class="field"><span class="label">Priority</span><input class="input" type="number" bind:value={workspaceStorage.priority} /></label><button class="button is-primary" type="submit" disabled={controller.state.savingAdministration}>Bind storage provider</button></form>
        {#if controller.state.administrationError !== ""}<p class="help is-danger" aria-live="polite">{controller.state.administrationError}</p>{/if}
        <div class="system-grant-list">{#each controller.state.workspaceAgents as binding (`${binding.workspace}/${binding.model}`)}<article class:system-grant-disabled={!binding.enabled} class="system-grant-row"><div><strong>{binding.workspace} / {binding.model}</strong><small>priority {binding.priority} / revision {binding.revision}</small></div><div class="system-grant-actions"><span>{binding.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.savingAdministration} onclick={() => void controller.saveAdministration(`workspace-agents/${encodeURIComponent(binding.workspace)}/${encodeURIComponent(binding.model)}`, "PATCH", { priority: binding.priority, label: binding.label, system_prompt: binding.system_prompt, enabled: !binding.enabled, expected_revision: binding.revision }, () => controller.loadAdministration("workspace-agents", "workspaceAgents"))}>{binding.enabled ? "Disable" : "Enable"}</button></div></article>{/each}{#each controller.state.workspaceStorageProviders as binding (`${binding.workspace}/${binding.provider}`)}<article class:system-grant-disabled={!binding.enabled} class="system-grant-row"><div><strong>{binding.workspace} / {binding.provider}</strong><small>storage priority {binding.priority} / revision {binding.revision}</small></div><div class="system-grant-actions"><span>{binding.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.savingAdministration} onclick={() => void controller.saveAdministration(`workspace-storage-providers/${encodeURIComponent(binding.workspace)}/${encodeURIComponent(binding.provider)}`, "PATCH", { priority: binding.priority, enabled: !binding.enabled, expected_revision: binding.revision }, () => controller.loadAdministration("workspace-storage-providers", "workspaceStorageProviders"))}>{binding.enabled ? "Disable" : "Enable"}</button></div></article>{/each}{#if controller.state.workspaceAgents.length === 0 && controller.state.workspaceStorageProviders.length === 0}<p class="dashboard-empty">No workspace bindings are configured.</p>{/if}</div>
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
