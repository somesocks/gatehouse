<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createAgentProviderFormController } from "./agent-provider-form-controller.svelte"

  let { providerID }: { providerID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() => createAgentProviderFormController({ onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))
  $effect(() => { void controller.load(providerID) })
</script>

<SystemFrame active="agent-providers" title="Agent provider">
  <section class="system-page">
    {#if controller.state.loading}
      <p class="dashboard-empty">Loading agent provider...</p>
    {:else if controller.state.provider === null}
      <p class="help is-danger" aria-live="polite">{controller.state.error === "" ? "Agent provider was not found." : controller.state.error}</p>
      <RouterLink class="button" href="/app/system/agent-providers">Back to providers</RouterLink>
    {:else if !controller.state.editing}
      <div class="system-page-heading mb-5"><div><p class="eyebrow">Agent provider</p><h2 class="title is-3">{controller.state.provider.alias}</h2><p class="subtitle is-6">{controller.state.provider.id}</p></div><button class="button is-primary" type="button" onclick={() => void controller.beginEdit()}>Edit</button></div>
      <dl>
        <div class="field"><dt class="label">Protocol</dt><dd class="control">{controller.state.provider.protocol}</dd></div>
        <div class="field"><dt class="label">Base URL</dt><dd class="control">{controller.state.provider.base_url ?? "Not configured"}</dd></div>
        <div class="field"><dt class="label">Keychain</dt><dd class="control">{controller.state.provider.keychain === undefined ? "Not configured" : `${controller.state.provider.keychain.id} / version ${controller.state.provider.keychain.version}`}</dd></div>
        <div class="field"><dt class="label">Credential</dt><dd class="control">{controller.state.provider.credential_configured ? "Configured" : "Not configured"}</dd></div>
        <div class="field"><dt class="label">Status</dt><dd class="control">{controller.state.provider.enabled ? "Enabled" : "Disabled"}</dd></div>
        <div class="field"><dt class="label">Revision</dt><dd class="control">{controller.state.provider.revision}</dd></div>
      </dl>
    {:else}
      <div class="system-page-heading mb-5"><div><p class="eyebrow">Agent provider</p><h2 class="title is-3">Edit {controller.state.provider.alias}</h2><p class="subtitle is-6">Update provider settings.</p></div></div>
      <form onsubmit={(event) => { event.preventDefault(); void controller.update() }}>
        <div class="field"><label class="label" for="agent-provider-alias">Alias</label><div class="control"><input class="input" id="agent-provider-alias" disabled value={controller.state.form.alias} /></div></div>
        <div class="field"><label class="label" for="agent-provider-protocol">Protocol</label><div class="control"><div class="select is-fullwidth"><select id="agent-provider-protocol" bind:value={controller.state.form.protocol}><option value="builtin">Built-in</option><option value="openai-chat-completions">OpenAI chat completions</option><option value="openai-responses">OpenAI responses</option></select></div></div></div>
        <div class="field"><label class="label" for="agent-provider-base-url">Base URL</label><div class="control"><input class="input" id="agent-provider-base-url" bind:value={controller.state.form.baseURL} /></div></div>
        {#if controller.state.form.protocol !== "builtin"}
          <div class="field"><label class="label" for="agent-provider-keychain">Keychain</label><div class="control"><div class="select is-fullwidth"><select id="agent-provider-keychain" required bind:value={controller.state.form.keychain}><option value="">Select keychain</option>{#each controller.state.keychains as keychain (`${keychain.id}/${keychain.version}`)}<option value={keychain.id}>{keychain.id} / version {keychain.version}</option>{/each}</select></div></div></div>
          <div class="field"><label class="label" for="agent-provider-api-key">Replacement API key</label><div class="control"><input class="input" id="agent-provider-api-key" type="password" autocomplete="new-password" bind:value={controller.state.form.apiKey} /></div><p class="help">Leave blank to preserve the current API key. Required when changing protocol or keychain.</p></div>
        {/if}
        <div class="field"><label class="checkbox"><input type="checkbox" bind:checked={controller.state.form.enabled} /> Enabled</label></div>
        <div class="field is-grouped"><p class="control"><button class="button is-primary" type="submit" disabled={controller.state.saving}>{controller.state.saving ? "Saving..." : "Save changes"}</button></p><p class="control"><button class="button" type="button" disabled={controller.state.saving} onclick={() => controller.state.editing = false}>Cancel</button></p></div>
      </form>
      {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    {/if}
  </section>
</SystemFrame>
