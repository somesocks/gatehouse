<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import SelectControl from "../../../components/SelectControl.svelte"
  import { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createStorageProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.keychains()
  })

  async function create(): Promise<void> {
    const provider = await controller.create()
    if (provider)
      runtime.navigate(
        `/app/system/storage-providers/${encodeURIComponent(provider.id)}`,
        true,
      )
  }
</script>

<SystemFrame active="storage-providers" title="New storage provider">
  <section class="stack">
    <div class="stack">
      <div>
        <p class="eyebrow">System</p>
        <h2>New storage provider</h2>
        <p>
          Credentials are write-only and cannot be viewed after saving.
        </p>
      </div>
    </div>
    <form
      class="stack"
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <div class="field">
        <label for="storage-provider-alias">Alias</label>
        <div>
          <input
            id="storage-provider-alias"
            required
            bind:value={controller.state.form.alias}
          />
        </div>
      </div>
      <div class="field">
        <label for="storage-provider-protocol">Protocol</label>
        <div>
          <div>
            <SelectControl>
              <select
                id="storage-provider-protocol"
                bind:value={controller.state.form.protocol}
                ><option value="embedded">Embedded</option><option value="s3"
                  >S3</option
                ></select
              >
            </SelectControl>
          </div>
        </div>
      </div>
      <div class="field">
        <label for="storage-provider-endpoint">Endpoint</label>
        <div>
          <input
            id="storage-provider-endpoint"
            bind:value={controller.state.form.endpoint}
          />
        </div>
      </div>
      {#if controller.state.form.protocol === "s3"}
        <div class="field">
          <label for="storage-provider-region">Region</label>
          <div>
            <input
              id="storage-provider-region"
              bind:value={controller.state.form.region}
            />
          </div>
        </div>
        <div class="field">
          <label for="storage-provider-bucket">Bucket</label>
          <div>
            <input
              id="storage-provider-bucket"
              bind:value={controller.state.form.bucket}
            />
          </div>
        </div>
        <div class="field">
          <label for="storage-provider-access-key-id"
            >Access key ID</label
          >
          <div>
            <input
              id="storage-provider-access-key-id"
              bind:value={controller.state.form.accessKeyID}
            />
          </div>
        </div>
        <div class="field">
          <label for="storage-provider-keychain">Keychain</label>
          <div>
            <div>
              <SelectControl>
                <select
                  id="storage-provider-keychain"
                  required
                  bind:value={controller.state.form.keychain}
                  ><option value="">Select keychain</option
                  >{#each controller.state.keychains as keychain (`${keychain.id}/${keychain.version}`)}<option
                      value={keychain.id}
                      >{keychain.id} / version {keychain.version}</option
                    >{/each}</select
                >
              </SelectControl>
            </div>
          </div>
        </div>
        <div class="field">
          <label for="storage-provider-secret"
            >Secret access key</label
          >
          <div>
            <input
              id="storage-provider-secret"
              type="password"
              required
              bind:value={controller.state.form.secret}
            />
          </div>
        </div>
      {/if}
      <div class="cluster">
        <div>
          <button class="primary" disabled={controller.state.saving}
            >Add provider</button
          >
        </div>
        <div>
          <RouterLink class="secondary" href="/app/system/storage-providers"
            >Cancel</RouterLink
          >
        </div>
      </div>
    </form>
    {#if controller.state.error}<p class="field-help" role="alert">
        {controller.state.error}
      </p>{/if}
  </section>
</SystemFrame>
