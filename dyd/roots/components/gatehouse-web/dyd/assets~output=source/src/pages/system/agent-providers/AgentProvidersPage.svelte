<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import { createAgentProvidersController } from "./agent-providers-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() => createAgentProvidersController({ activity: runtime.activity, onAuthenticationLost: runtime.requireLogin, onSystemAccessChange: runtime.access.setSystemAccess }))

  $effect(() => { void controller.load() })
  $effect(() => controller.start())
</script>

<SystemFrame active="agent-providers" title="Agent providers">
  <section class="system-page">
    <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Agent providers</h2><p class="subtitle is-6">Credentials are write-only. Leave the API key blank when updating an existing provider.</p></div></div>
    <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void controller.create() }}>
      <label class="field"><span class="label">Alias</span><input class="input" required bind:value={controller.state.form.alias} /></label><label class="field"><span class="label">Protocol</span><select class="select" bind:value={controller.state.form.protocol}><option value="builtin">Built-in</option><option value="openai-chat-completions">OpenAI chat completions</option><option value="openai-responses">OpenAI responses</option></select></label><label class="field"><span class="label">Base URL</span><input class="input" bind:value={controller.state.form.baseURL} /></label><label class="field"><span class="label">Keychain ID</span><input class="input" bind:value={controller.state.form.keychain} /></label><label class="field"><span class="label">API key</span><input class="input" type="password" autocomplete="new-password" bind:value={controller.state.form.apiKey} /></label><button class="button is-primary" type="submit" disabled={controller.state.saving}>Add provider</button>
    </form>
    {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    <div class="system-grant-list">{#each controller.state.providers as provider (provider.id)}<article class:system-grant-disabled={!provider.enabled} class="system-grant-row"><div><strong>{provider.alias}</strong><small>{provider.id} / {provider.protocol} / revision {provider.revision}{provider.credential_configured ? " / credential configured" : ""}</small></div><div class="system-grant-actions"><span>{provider.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.saving} onclick={() => void controller.setEnabled(provider, !provider.enabled)}>{provider.enabled ? "Disable" : "Enable"}</button></div></article>{:else}<p class="dashboard-empty">No agent providers are configured.</p>{/each}</div>
  </section>
</SystemFrame>
