<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import SelectControl from "../../../components/SelectControl.svelte"
  import { createAgentProviderFormController } from "./agent-provider-form-controller.svelte"

  let { providerID }: { providerID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.load(providerID)
  })
</script>

<SystemFrame active="agent-providers" title="Agent provider">
  <section class="stack">
    {#if controller.state.loading}
      <p class="muted">Loading agent provider...</p>
    {:else if controller.state.provider === null}
      <p class="field-help" role="alert" aria-live="polite">
        {controller.state.error === ""
          ? "Agent provider was not found."
          : controller.state.error}
      </p>
      <RouterLink class="secondary" href="/app/system/agent-providers"
        >Back to providers</RouterLink
      >
    {:else if !controller.state.editing}
      <div class="split">
        <div>
          <p class="eyebrow">Agent provider</p>
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
          <dt>Base URL</dt>
          <dd>
            {controller.state.provider.base_url ?? "Not configured"}
          </dd>
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
          <dd>
            {controller.state.provider.enabled ? "Enabled" : "Disabled"}
          </dd>
        </div>
        <div class="field">
          <dt>Revision</dt>
          <dd>{controller.state.provider.revision}</dd>
        </div>
      </dl>
    {:else}
      <div class="stack">
        <div>
          <p class="eyebrow">Agent provider</p>
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
          <label for="agent-provider-alias">Alias</label>
          <div>
            <input
              id="agent-provider-alias"
              disabled
              value={controller.state.form.alias}
            />
          </div>
        </div>
        <div class="field">
          <label for="agent-provider-protocol">Protocol</label>
          <div>
            <div>
              <SelectControl>
                <select
                  id="agent-provider-protocol"
                  bind:value={controller.state.form.protocol}
                  ><option value="builtin">Built-in</option><option
                    value="openai-chat-completions"
                    >OpenAI chat completions</option
                  ><option value="openai-responses">OpenAI responses</option
                  ></select
                >
              </SelectControl>
            </div>
          </div>
        </div>
        <div class="field">
          <label for="agent-provider-base-url">Base URL</label>
          <div>
            <input
              id="agent-provider-base-url"
              bind:value={controller.state.form.baseURL}
            />
          </div>
        </div>
        {#if controller.state.form.protocol !== "builtin"}
          <div class="field">
            <label for="agent-provider-keychain">Keychain</label>
            <div>
              <div>
                <SelectControl>
                  <select
                    id="agent-provider-keychain"
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
            <label for="agent-provider-api-key"
              >Replacement API key</label
            >
            <div>
              <input
                id="agent-provider-api-key"
                type="password"
                autocomplete="new-password"
                bind:value={controller.state.form.apiKey}
              />
            </div>
            <p class="field-help">
              Leave blank to preserve the current API key. Required when
              changing protocol or keychain.
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
            <button
              class="primary"
              type="submit"
              disabled={controller.state.saving}
              >{controller.state.saving ? "Saving..." : "Save changes"}</button
            >
          </div>
          <div>
            <button
              class="secondary"
              type="button"
              disabled={controller.state.saving}
              onclick={() => (controller.state.editing = false)}>Cancel</button
            >
          </div>
        </div>
      </form>
      {#if controller.state.error !== ""}<p
          class="field-help"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </p>{/if}
    {/if}
  </section>
</SystemFrame>
