<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"

  let { providerID }: { providerID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() => createStorageProviderFormController({ onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))
  $effect(() => { void controller.load(providerID) })
</script>

<SystemFrame active="storage-providers" title="Storage provider">
  <section class="system-page">
    {#if controller.state.loading}
      <p class="dashboard-empty">Loading storage provider...</p>
    {:else if controller.state.provider === null}
      <p class="help is-danger">{controller.state.error || "Storage provider was not found."}</p>
      <RouterLink class="button" href="/app/system/storage-providers">Back to providers</RouterLink>
    {:else if !controller.state.editing}
      <div class="system-page-heading mb-5"><div><p class="eyebrow">Storage provider</p><h2 class="title is-3">{controller.state.provider.alias}</h2><p class="subtitle is-6">{controller.state.provider.id}</p></div><button class="button is-primary" type="button" onclick={() => void controller.beginEdit()}>Edit</button></div>
      <dl><div class="field"><dt class="label">Protocol</dt><dd>{controller.state.provider.protocol}</dd></div><div class="field"><dt class="label">Endpoint</dt><dd>{controller.state.provider.endpoint ?? "Not configured"}</dd></div><div class="field"><dt class="label">Region</dt><dd>{controller.state.provider.region ?? "Not configured"}</dd></div><div class="field"><dt class="label">Bucket</dt><dd>{controller.state.provider.bucket ?? "Not configured"}</dd></div><div class="field"><dt class="label">Access key ID</dt><dd>{controller.state.provider.access_key_id ?? "Not configured"}</dd></div><div class="field"><dt class="label">Keychain</dt><dd>{controller.state.provider.keychain === undefined ? "Not configured" : `${controller.state.provider.keychain.id} / version ${controller.state.provider.keychain.version}`}</dd></div><div class="field"><dt class="label">Credential</dt><dd>{controller.state.provider.credential_configured ? "Configured" : "Not configured"}</dd></div><div class="field"><dt class="label">Status</dt><dd>{controller.state.provider.enabled ? "Enabled" : "Disabled"}</dd></div></dl>
    {:else}
      <div class="system-page-heading mb-5"><div><p class="eyebrow">Storage provider</p><h2 class="title is-3">Edit {controller.state.provider.alias}</h2><p class="subtitle is-6">Update provider settings.</p></div></div>
      <form onsubmit={(event) => { event.preventDefault(); void controller.update() }}>
        <div class="field"><label class="label" for="storage-provider-alias">Alias</label><div class="control"><input class="input" id="storage-provider-alias" disabled value={controller.state.form.alias} /></div></div>
        <div class="field"><label class="label" for="storage-provider-protocol">Protocol</label><div class="control"><div class="select is-fullwidth"><select id="storage-provider-protocol" bind:value={controller.state.form.protocol}><option value="embedded">Embedded</option><option value="s3">S3</option></select></div></div></div>
        <div class="field"><label class="label" for="storage-provider-endpoint">Endpoint</label><div class="control"><input class="input" id="storage-provider-endpoint" bind:value={controller.state.form.endpoint} /></div></div>
        {#if controller.state.form.protocol === "s3"}
          <div class="field"><label class="label" for="storage-provider-region">Region</label><div class="control"><input class="input" id="storage-provider-region" bind:value={controller.state.form.region} /></div></div>
          <div class="field"><label class="label" for="storage-provider-bucket">Bucket</label><div class="control"><input class="input" id="storage-provider-bucket" bind:value={controller.state.form.bucket} /></div></div>
          <div class="field"><label class="label" for="storage-provider-access-key-id">Access key ID</label><div class="control"><input class="input" id="storage-provider-access-key-id" bind:value={controller.state.form.accessKeyID} /></div></div>
          <div class="field"><label class="label" for="storage-provider-keychain">Keychain</label><div class="control"><div class="select is-fullwidth"><select id="storage-provider-keychain" required bind:value={controller.state.form.keychain}><option value="">Select keychain</option>{#each controller.state.keychains as keychain (`${keychain.id}/${keychain.version}`)}<option value={keychain.id}>{keychain.id} / version {keychain.version}</option>{/each}</select></div></div></div>
          <div class="field"><label class="label" for="storage-provider-secret">Replacement secret access key</label><div class="control"><input class="input" id="storage-provider-secret" type="password" bind:value={controller.state.form.secret} /></div><p class="help">Leave blank to preserve the current secret. Required when changing protocol or keychain.</p></div>
        {/if}
        <div class="field"><label class="checkbox"><input type="checkbox" bind:checked={controller.state.form.enabled} /> Enabled</label></div>
        <div class="field is-grouped"><p class="control"><button class="button is-primary" disabled={controller.state.saving}>Save changes</button></p><p class="control"><button class="button" type="button" onclick={() => controller.state.editing = false}>Cancel</button></p></div>
      </form>
      {#if controller.state.error}<p class="help is-danger">{controller.state.error}</p>{/if}
    {/if}
  </section>
</SystemFrame>
