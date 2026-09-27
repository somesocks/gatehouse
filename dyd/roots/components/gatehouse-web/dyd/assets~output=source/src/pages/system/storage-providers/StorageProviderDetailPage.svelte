<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import SelectControl from "../../../components/SelectControl.svelte"
  import { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"

  let { providerID }: { providerID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createStorageProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.load(providerID)
  })
</script>

<SystemFrame active="storage-providers" title="Storage provider">
  <section class="stack">
    {#if controller.state.loading}
      <p class="muted">Loading storage provider...</p>
    {:else if controller.state.provider === null}
      <p class="field-help" role="alert">
        {controller.state.error || "Storage provider was not found."}
      </p>
      <RouterLink class="secondary" href="/app/system/storage-providers"
        >Back to providers</RouterLink
      >
    {:else if !controller.state.editing}
      <div class="split">
        <div>
          <p class="eyebrow">Storage provider</p>
          <h2>{controller.state.provider.alias}</h2>
          <p>{controller.state.provider.id}</p>
        </div>
        <button
          class="secondary"
          type="button"
          onclick={() => void controller.beginEdit()}>Edit</button
        >
      </div>
      <dl>
        <div class="field">
          <dt>Protocol</dt>
          <dd>{controller.state.provider.protocol}</dd>
        </div>
        <div class="field">
          <dt>Endpoint</dt>
          <dd>{controller.state.provider.endpoint ?? "Not configured"}</dd>
        </div>
        <div class="field">
          <dt>Region</dt>
          <dd>{controller.state.provider.region ?? "Not configured"}</dd>
        </div>
        <div class="field">
          <dt>Bucket</dt>
          <dd>{controller.state.provider.bucket ?? "Not configured"}</dd>
        </div>
        <div class="field">
          <dt>Access key ID</dt>
          <dd>{controller.state.provider.access_key_id ?? "Not configured"}</dd>
        </div>
        <div class="field">
          <dt>Keychain</dt>
          <dd>
            {controller.state.provider.keychain === undefined
              ? "Not configured"
              : `${controller.state.provider.keychain.id} / version ${controller.state.provider.keychain.version}`}
          </dd>
        </div>
        <div class="field">
          <dt>Credential</dt>
          <dd>
            {controller.state.provider.credential_configured
              ? "Configured"
              : "Not configured"}
          </dd>
        </div>
        <div class="field">
          <dt>Status</dt>
          <dd>{controller.state.provider.enabled ? "Enabled" : "Disabled"}</dd>
        </div>
      </dl>
    {:else}
      <div class="stack">
        <div>
          <p class="eyebrow">Storage provider</p>
          <h2>Edit {controller.state.provider.alias}</h2>
          <p>Update provider settings.</p>
        </div>
      </div>
      <form
        class="stack"
        onsubmit={(event) => {
          event.preventDefault()
          void controller.update()
        }}
      >
        <div class="field">
          <label for="storage-provider-alias">Alias</label>
          <div>
            <input
              id="storage-provider-alias"
              disabled
              value={controller.state.form.alias}
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
            <label for="storage-provider-keychain">Keychain</label
            >
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
              >Replacement secret access key</label
            >
            <div>
              <input
                id="storage-provider-secret"
                type="password"
                bind:value={controller.state.form.secret}
              />
            </div>
            <p class="field-help">
              Leave blank to preserve the current secret. Required when changing
              protocol or keychain.
            </p>
          </div>
        {/if}
        <div class="field">
          <label class="choice"
            ><input
              type="checkbox"
              bind:checked={controller.state.form.enabled}
            /> Enabled</label
          >
        </div>
        <div class="cluster">
          <div>
            <button class="primary" disabled={controller.state.saving}
              >Save changes</button
            >
          </div>
          <div>
            <button
              class="secondary"
              type="button"
              onclick={() => (controller.state.editing = false)}>Cancel</button
            >
          </div>
        </div>
      </form>
      {#if controller.state.error}<p class="field-help" role="alert">
          {controller.state.error}
        </p>{/if}
    {/if}
  </section>
</SystemFrame>
