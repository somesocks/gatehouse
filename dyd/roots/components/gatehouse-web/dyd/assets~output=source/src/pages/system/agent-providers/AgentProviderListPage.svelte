<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createAgentProviderListController } from "./agent-provider-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentProviderListController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  const matches = (
    provider: { id: string; alias: string; protocol: string },
    search: string,
  ) => {
    const query = search.trim().toLocaleLowerCase()
    return (
      query === "" ||
      `${provider.id} ${provider.alias} ${provider.protocol}`
        .toLocaleLowerCase()
        .includes(query)
    )
  }
  $effect(() => {
    void controller.load()
  })
  $effect(() => controller.start())
</script>

<SystemFrame active="agent-providers" title="Agent providers">
  <section class="stack">
    <div class="split">
      <div>
        <p class="eyebrow">System</p>
        <h2>Agent providers</h2>
        <p>
          Configure model-provider protocols and credentials.
        </p>
      </div>
      <RouterLink
        class="primary"
        href="/app/system/agent-providers/new">Add provider</RouterLink
      >
    </div>
    <div class="field">
      <label
        ><span>Search agent providers</span><input
          type="search"
          autocomplete="off"
          placeholder="Search by ID, alias, or protocol"
          bind:value={controller.state.search}
        /></label
      >
    </div>
    {#if controller.state.error !== ""}<p
        class="field-help"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error}
      </p>{/if}
    <div class="list">
      {#each controller.state.providers.filter( (provider) => matches(provider, controller.state.search), ) as provider (provider.id)}<RouterLink
          class="list-item surface split"
          data-disabled={!provider.enabled || undefined}
          href={`/app/system/agent-providers/${encodeURIComponent(provider.id)}`}
          ><div class="stack">
            <strong>{provider.alias}</strong><small
              >{provider.id} / {provider.protocol} / revision {provider.revision}{provider.credential_configured
                ? " / credential configured"
                : ""}</small
            >
          </div>
          <span>{provider.enabled ? "Enabled" : "Disabled"}</span></RouterLink
        >{:else}<p class="muted">
          No agent providers match your search.
        </p>{/each}
    </div>
  </section>
</SystemFrame>
