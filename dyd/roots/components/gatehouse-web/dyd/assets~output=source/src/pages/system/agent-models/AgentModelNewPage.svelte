<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createAgentModelFormController } from "./agent-model-form-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentModelFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.loadProviders()
  })
  async function create(): Promise<void> {
    const model = await controller.create()
    if (model !== null)
      runtime.navigate(
        `/app/system/agent-models/${encodeURIComponent(model.id)}`,
        true,
      )
  }
</script>

<SystemFrame active="agent-models" title="New agent model"
  ><section class="stack">
    <div class="stack">
      <div>
        <p class="eyebrow">System</p>
        <h2>New agent model</h2>
        <p>
          Configure a model available for workspace bindings.
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
        <label for="agent-model-alias">Alias</label>
        <div>
          <input
            id="agent-model-alias"
            required
            bind:value={controller.state.form.alias}
          />
        </div>
      </div>
      <div class="field">
        <label for="agent-model-provider">Provider</label>
        <div>
          <div>
            <select
              id="agent-model-provider"
              required
              bind:value={controller.state.form.provider}
              ><option value="">Select provider</option
              >{#each controller.state.providers as provider (provider.id)}<option
                  value={provider.id}
                  disabled={!provider.enabled}
                  >{provider.alias} / {provider.id}{provider.enabled
                    ? ""
                    : " / Disabled"}</option
                >{/each}</select
            >
          </div>
        </div>
      </div>
      <div class="field">
        <label for="agent-model-name">Model</label>
        <div>
          <input
            id="agent-model-name"
            required
            bind:value={controller.state.form.model}
          />
        </div>
      </div>
      <div class="field">
        <label for="agent-model-parameters">Parameters</label>
        <div>
          <input
            id="agent-model-parameters"
            bind:value={controller.state.form.parameters}
          />
        </div>
      </div>
      <div class="field">
        <label for="agent-model-compaction">Compaction</label>
        <div>
          <input
            id="agent-model-compaction"
            bind:value={controller.state.form.compaction}
          />
        </div>
      </div>
      <div class="field">
        <label for="agent-model-max-turns">Max turns</label>
        <div>
          <input
            id="agent-model-max-turns"
            type="number"
            min="0"
            bind:value={controller.state.form.maxTurns}
          />
        </div>
      </div>
      <div class="field">
        <label for="agent-model-max-output-tokens"
          >Max output tokens</label
        >
        <div>
          <input
            id="agent-model-max-output-tokens"
            type="number"
            min="0"
            bind:value={controller.state.form.maxOutputTokens}
          />
        </div>
      </div>
      <div class="cluster">
        <div>
          <button
            class="primary"
            type="submit"
            disabled={controller.state.saving}
            >{controller.state.saving ? "Saving..." : "Add model"}</button
          >
        </div>
        <div>
          <RouterLink class="secondary" href="/app/system/agent-models"
            >Cancel</RouterLink
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
  </section></SystemFrame
>
