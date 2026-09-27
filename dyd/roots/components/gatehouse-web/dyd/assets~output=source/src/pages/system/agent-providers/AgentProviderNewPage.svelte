<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import SelectControl from "../../../components/SelectControl.svelte"
  import { createAgentProviderFormController } from "./agent-provider-form-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.keychains()
  })
  async function create(): Promise<void> {
    const provider = await controller.create()
    if (provider !== null)
      runtime.navigate(
        `/app/system/agent-providers/${encodeURIComponent(provider.id)}`,
        true,
      )
  }
</script>

<SystemFrame active="agent-providers" title="New agent provider">
  <section class="stack">
    <div class="stack">
      <div>
        <p class="eyebrow">System</p>
        <h2>New agent provider</h2>
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
      <label class="field"
        ><span>Alias</span><input
          required
          bind:value={controller.state.form.alias}
        /></label
      ><label class="field"
        ><span>Protocol</span><SelectControl><select
          bind:value={controller.state.form.protocol}
          ><option value="builtin">Built-in</option><option
            value="openai-chat-completions">OpenAI chat completions</option
          ><option value="openai-responses">OpenAI responses</option></select
        ></SelectControl></label
      ><label class="field"
        ><span>Base URL</span><input
          bind:value={controller.state.form.baseURL}
        /></label
      >{#if controller.state.form.protocol !== "builtin"}<label class="field"
          ><span>Keychain</span><SelectControl><select
            required
            bind:value={controller.state.form.keychain}
            ><option value="">Select keychain</option
            >{#each controller.state.keychains as keychain (`${keychain.id}/${keychain.version}`)}<option
                value={keychain.id}
                >{keychain.id} / version {keychain.version}</option
              >{/each}</select
          ></SelectControl></label
        ><label class="field"
          ><span>API key</span><input
            type="password"
            autocomplete="new-password"
            required
            bind:value={controller.state.form.apiKey}
          /></label
        >{/if}<button
        class="primary"
        type="submit"
        disabled={controller.state.saving}
        >{controller.state.saving ? "Saving..." : "Add provider"}</button
      ><RouterLink class="secondary" href="/app/system/agent-providers"
        >Cancel</RouterLink
      >
    </form>
    {#if controller.state.error !== ""}<p
        class="field-help"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error}
      </p>{/if}
  </section>
</SystemFrame>
