<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() => createStorageProviderFormController({ onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))
  $effect(() => { void controller.keychains() })

  async function create(): Promise<void> {
    const provider = await controller.create()
    if (provider) runtime.navigate(`/app/system/storage-providers/${encodeURIComponent(provider.id)}`, true)
  }
</script>

<SystemFrame active="storage-providers" title="New storage provider">
  <section class="system-page">
    <div class="system-page-heading mb-5"><div><p class="eyebrow">System</p><h2 class="title is-3">New storage provider</h2><p class="subtitle is-6">Credentials are write-only and cannot be viewed after saving.</p></div></div>
    <form onsubmit={(event) => { event.preventDefault(); void create() }}>
      <div class="field"><label class="label" for="storage-provider-alias">Alias</label><div class="control"><input class="input" id="storage-provider-alias" required bind:value={controller.state.form.alias} /></div></div>
      <div class="field"><label class="label" for="storage-provider-protocol">Protocol</label><div class="control"><div class="select is-fullwidth"><select id="storage-provider-protocol" bind:value={controller.state.form.protocol}><option value="embedded">Embedded</option><option value="s3">S3</option></select></div></div></div>
      <div class="field"><label class="label" for="storage-provider-endpoint">Endpoint</label><div class="control"><input class="input" id="storage-provider-endpoint" bind:value={controller.state.form.endpoint} /></div></div>
      {#if controller.state.form.protocol === "s3"}
        <div class="field"><label class="label" for="storage-provider-region">Region</label><div class="control"><input class="input" id="storage-provider-region" bind:value={controller.state.form.region} /></div></div>
        <div class="field"><label class="label" for="storage-provider-bucket">Bucket</label><div class="control"><input class="input" id="storage-provider-bucket" bind:value={controller.state.form.bucket} /></div></div>
        <div class="field"><label class="label" for="storage-provider-access-key-id">Access key ID</label><div class="control"><input class="input" id="storage-provider-access-key-id" bind:value={controller.state.form.accessKeyID} /></div></div>
        <div class="field"><label class="label" for="storage-provider-keychain">Keychain</label><div class="control"><div class="select is-fullwidth"><select id="storage-provider-keychain" required bind:value={controller.state.form.keychain}><option value="">Select keychain</option>{#each controller.state.keychains as keychain (`${keychain.id}/${keychain.version}`)}<option value={keychain.id}>{keychain.id} / version {keychain.version}</option>{/each}</select></div></div></div>
        <div class="field"><label class="label" for="storage-provider-secret">Secret access key</label><div class="control"><input class="input" id="storage-provider-secret" type="password" required bind:value={controller.state.form.secret} /></div></div>
      {/if}
      <div class="field is-grouped"><p class="control"><button class="button is-primary" disabled={controller.state.saving}>Add provider</button></p><p class="control"><RouterLink class="button" href="/app/system/storage-providers">Cancel</RouterLink></p></div>
    </form>
    {#if controller.state.error}<p class="help is-danger">{controller.state.error}</p>{/if}
  </section>
</SystemFrame>
