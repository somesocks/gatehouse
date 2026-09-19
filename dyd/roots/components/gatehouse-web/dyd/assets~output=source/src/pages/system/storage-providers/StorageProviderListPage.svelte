<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createStorageProviderListController } from "./storage-provider-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createStorageProviderListController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  const matches = (
    p: { id: string; alias: string; protocol: string; bucket?: string },
    s: string,
  ) => {
    const q = s.trim().toLocaleLowerCase()
    return (
      q === "" ||
      `${p.id} ${p.alias} ${p.protocol} ${p.bucket ?? ""}`
        .toLocaleLowerCase()
        .includes(q)
    )
  }
  $effect(() => {
    void controller.load()
  })
  $effect(() => controller.start())
</script>

<SystemFrame active="storage-providers" title="Storage providers"
  ><section class="stack">
    <div class="split">
      <div>
        <p class="eyebrow">System</p>
        <h2>Storage providers</h2>
        <p>Configure embedded and S3 storage.</p>
      </div>
      <RouterLink
        class="primary"
        href="/app/system/storage-providers/new">Add provider</RouterLink
      >
    </div>
    <div class="field">
      <label
        ><span>Search storage providers</span><input
          type="search"
          bind:value={controller.state.search}
        /></label
      >
    </div>
    {#if controller.state.error}<p class="field-help" role="alert">
        {controller.state.error}
      </p>{/if}
    <div class="list">
      {#each controller.state.providers.filter( (p) => matches(p, controller.state.search), ) as p (p.id)}<RouterLink
          class="list-item surface split"
          data-disabled={!p.enabled || undefined}
          href={`/app/system/storage-providers/${encodeURIComponent(p.id)}`}
          ><div class="stack">
            <strong>{p.alias}</strong><small
              >{p.id} / {p.protocol} / {p.bucket ?? "no bucket"}</small
            >
          </div>
          <span>{p.enabled ? "Enabled" : "Disabled"}</span></RouterLink
        >{:else}<p class="muted">
          No storage providers match your search.
        </p>{/each}
    </div>
  </section></SystemFrame
>
