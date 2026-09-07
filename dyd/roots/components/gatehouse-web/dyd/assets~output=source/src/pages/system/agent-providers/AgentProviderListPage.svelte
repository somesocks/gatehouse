<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createAgentProviderListController } from "./agent-provider-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() => createAgentProviderListController({ activity: runtime.activity, onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))
  const matches = (provider: { id: string; alias: string; protocol: string }, search: string) => { const query = search.trim().toLocaleLowerCase(); return query === "" || `${provider.id} ${provider.alias} ${provider.protocol}`.toLocaleLowerCase().includes(query) }
  $effect(() => { void controller.load() })
  $effect(() => controller.start())
</script>

<SystemFrame active="agent-providers" title="Agent providers">
  <section class="system-page"><div class="system-page-heading mb-5"><div><p class="eyebrow">System</p><h2 class="title is-3">Agent providers</h2><p class="subtitle is-6">Configure model-provider protocols and credentials.</p></div><RouterLink class="button is-primary" href="/app/system/agent-providers/new">Add provider</RouterLink></div>
    <div class="collection-search"><label><span>Search agent providers</span><input class="input" type="search" autocomplete="off" placeholder="Search by ID, alias, or protocol" bind:value={controller.state.search} /></label></div>
    {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    <div class="system-grant-list">{#each controller.state.providers.filter((provider) => matches(provider, controller.state.search)) as provider (provider.id)}<RouterLink class={`system-grant-row${provider.enabled ? "" : " system-grant-disabled"}`} href={`/app/system/agent-providers/${encodeURIComponent(provider.id)}`}><div><strong>{provider.alias}</strong><small>{provider.id} / {provider.protocol} / revision {provider.revision}{provider.credential_configured ? " / credential configured" : ""}</small></div><span>{provider.enabled ? "Enabled" : "Disabled"}</span></RouterLink>{:else}<p class="dashboard-empty">No agent providers match your search.</p>{/each}</div>
  </section>
</SystemFrame>
