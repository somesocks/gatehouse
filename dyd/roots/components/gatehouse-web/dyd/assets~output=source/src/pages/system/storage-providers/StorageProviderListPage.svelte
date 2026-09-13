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
  ><section class="system-page">
    <div class="system-page-heading mb-5">
      <div>
        <p class="eyebrow">System</p>
        <h2 class="title is-3">Storage providers</h2>
        <p class="subtitle is-6">Configure embedded and S3 storage.</p>
      </div>
      <RouterLink
        class="button is-primary"
        href="/app/system/storage-providers/new">Add provider</RouterLink
      >
    </div>
    <div class="collection-search">
      <label
        ><span>Search storage providers</span><input
          class="input"
          type="search"
          bind:value={controller.state.search}
        /></label
      >
    </div>
    {#if controller.state.error}<p class="help is-danger">
        {controller.state.error}
      </p>{/if}
    <div class="system-grant-list">
      {#each controller.state.providers.filter( (p) => matches(p, controller.state.search), ) as p (p.id)}<RouterLink
          class={`system-grant-row${p.enabled ? "" : " system-grant-disabled"}`}
          href={`/app/system/storage-providers/${encodeURIComponent(p.id)}`}
          ><div>
            <strong>{p.alias}</strong><small
              >{p.id} / {p.protocol} / {p.bucket ?? "no bucket"}</small
            >
          </div>
          <span>{p.enabled ? "Enabled" : "Disabled"}</span></RouterLink
        >{:else}<p class="dashboard-empty">
          No storage providers match your search.
        </p>{/each}
    </div>
  </section></SystemFrame
>
