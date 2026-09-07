<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import { createStorageProvidersController } from "./storage-providers-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() => createStorageProvidersController({ activity: runtime.activity, onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))

  $effect(() => { void controller.load() })
  $effect(() => controller.start())
</script>

<SystemFrame active="storage-providers" title="Storage providers">
  <section class="system-page">
    <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Storage providers</h2><p class="subtitle is-6">Credentials are write-only. Embedded storage needs no credentials.</p></div></div>
    <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void controller.create() }}>
      <label class="field"><span class="label">Alias</span><input class="input" required bind:value={controller.state.form.alias} /></label><label class="field"><span class="label">Protocol</span><select class="select" bind:value={controller.state.form.protocol}><option value="embedded">Embedded</option><option value="s3">S3</option></select></label><label class="field"><span class="label">Endpoint</span><input class="input" bind:value={controller.state.form.endpoint} /></label><label class="field"><span class="label">Region</span><input class="input" bind:value={controller.state.form.region} /></label><label class="field"><span class="label">Bucket</span><input class="input" bind:value={controller.state.form.bucket} /></label><label class="field"><span class="label">Access key ID</span><input class="input" bind:value={controller.state.form.accessKeyID} /></label><label class="field"><span class="label">Keychain ID</span><input class="input" bind:value={controller.state.form.keychain} /></label><label class="field"><span class="label">Secret access key</span><input class="input" type="password" autocomplete="new-password" bind:value={controller.state.form.secretAccessKey} /></label><button class="button is-primary" type="submit" disabled={controller.state.saving}>Add provider</button>
    </form>
    {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    <div class="system-grant-list">{#each controller.state.providers as provider (provider.id)}<article class:system-grant-disabled={!provider.enabled} class="system-grant-row"><div><strong>{provider.alias}</strong><small>{provider.id} / {provider.protocol} / {provider.bucket ?? "no bucket"} / revision {provider.revision}{provider.credential_configured ? " / credential configured" : ""}</small></div><div class="system-grant-actions"><span>{provider.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.saving} onclick={() => void controller.setEnabled(provider, !provider.enabled)}>{provider.enabled ? "Disable" : "Enable"}</button></div></article>{:else}<p class="dashboard-empty">No storage providers are configured.</p>{/each}</div>
  </section>
</SystemFrame>
