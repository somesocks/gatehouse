<script lang="ts">
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../../app/activity"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import { createAgentModelsController } from "./agent-models-controller.svelte"

  let { activity, onAuthenticationLost, onSystemAccessChange }: { activity: ActivityClient; onAuthenticationLost: () => void; onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void } = $props()
  const controller = untrack(() => createAgentModelsController({ activity, onAuthenticationLost, onSystemAccessChange }))

  $effect(() => { void controller.load() })
  $effect(() => controller.start())
</script>

<SystemFrame active="agent-models" title="Agent models">
  <section class="system-page">
    <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Agent models</h2><p class="subtitle is-6">Models with the same workspace priority are selected randomly.</p></div></div>
    <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void controller.create() }}>
      <label class="field"><span class="label">Alias</span><input class="input" required bind:value={controller.state.form.alias} /></label><label class="field"><span class="label">Provider ID</span><input class="input" required bind:value={controller.state.form.provider} /></label><label class="field"><span class="label">Model</span><input class="input" required bind:value={controller.state.form.model} /></label><label class="field"><span class="label">Parameters</span><input class="input" bind:value={controller.state.form.parameters} /></label><label class="field"><span class="label">Compaction</span><input class="input" bind:value={controller.state.form.compaction} /></label><label class="field"><span class="label">Max turns</span><input class="input" type="number" min="0" bind:value={controller.state.form.maxTurns} /></label><label class="field"><span class="label">Max output tokens</span><input class="input" type="number" min="0" bind:value={controller.state.form.maxOutputTokens} /></label><button class="button is-primary" type="submit" disabled={controller.state.saving}>Add model</button>
    </form>
    {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    <div class="system-grant-list">{#each controller.state.models as model (model.id)}<article class:system-grant-disabled={!model.enabled} class="system-grant-row"><div><strong>{model.alias}</strong><small>{model.id} / {model.provider} / {model.model} / revision {model.revision}</small></div><div class="system-grant-actions"><span>{model.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={controller.state.saving} onclick={() => void controller.setEnabled(model, !model.enabled)}>{model.enabled ? "Disable" : "Enable"}</button></div></article>{:else}<p class="dashboard-empty">No agent models are configured.</p>{/each}</div>
  </section>
</SystemFrame>
