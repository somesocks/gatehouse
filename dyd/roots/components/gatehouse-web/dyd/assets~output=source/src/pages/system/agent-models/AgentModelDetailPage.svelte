<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createAgentModelFormController } from "./agent-model-form-controller.svelte"
  let { modelID }: { modelID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentModelFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.load(modelID)
  })
</script>

<SystemFrame active="agent-models" title="Agent model"
  ><section class="system-page">
    {#if controller.state.loading}<p class="dashboard-empty">
        Loading agent model...
      </p>{:else if controller.state.model === null}<p
        class="help is-danger"
        aria-live="polite"
      >
        {controller.state.error === ""
          ? "Agent model was not found."
          : controller.state.error}
      </p>
      <RouterLink class="button" href="/app/system/agent-models"
        >Back to models</RouterLink
      >{:else if !controller.state.editing}<div
        class="system-page-heading mb-5"
      >
        <div>
          <p class="eyebrow">Agent model</p>
          <h2 class="title is-3">{controller.state.model.alias}</h2>
          <p class="subtitle is-6">{controller.state.model.id}</p>
        </div>
        <button
          class="button is-primary"
          type="button"
          onclick={() => void controller.beginEdit()}>Edit</button
        >
      </div>
      <dl>
        <div class="field">
          <dt class="label">Provider</dt>
          <dd class="control">{controller.state.model.provider}</dd>
        </div>
        <div class="field">
          <dt class="label">Model</dt>
          <dd class="control">{controller.state.model.model}</dd>
        </div>
        <div class="field">
          <dt class="label">Parameters</dt>
          <dd class="control">
            {controller.state.model.parameters === ""
              ? "Not configured"
              : controller.state.model.parameters}
          </dd>
        </div>
        <div class="field">
          <dt class="label">Compaction</dt>
          <dd class="control">
            {controller.state.model.compaction === ""
              ? "Not configured"
              : controller.state.model.compaction}
          </dd>
        </div>
        <div class="field">
          <dt class="label">Max turns</dt>
          <dd class="control">{controller.state.model.max_turns}</dd>
        </div>
        <div class="field">
          <dt class="label">Max output tokens</dt>
          <dd class="control">{controller.state.model.max_output_tokens}</dd>
        </div>
        <div class="field">
          <dt class="label">Status</dt>
          <dd class="control">
            {controller.state.model.enabled ? "Enabled" : "Disabled"}
          </dd>
        </div>
        <div class="field">
          <dt class="label">Revision</dt>
          <dd class="control">{controller.state.model.revision}</dd>
        </div>
      </dl>{:else}<div class="system-page-heading mb-5">
        <div>
          <p class="eyebrow">Agent model</p>
          <h2 class="title is-3">Edit {controller.state.model.alias}</h2>
          <p class="subtitle is-6">Update model settings.</p>
        </div>
      </div>
      <form
        onsubmit={(event) => {
          event.preventDefault()
          void controller.update()
        }}
      >
        <div class="field">
          <label class="label" for="edit-agent-model-alias">Alias</label>
          <div class="control">
            <input
              class="input"
              id="edit-agent-model-alias"
              disabled
              value={controller.state.form.alias}
            />
          </div>
        </div>
        <div class="field">
          <label class="label" for="edit-agent-model-provider">Provider</label>
          <div class="control">
            <div class="select is-fullwidth">
              <select
                id="edit-agent-model-provider"
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
          <label class="label" for="edit-agent-model-name">Model</label>
          <div class="control">
            <input
              class="input"
              id="edit-agent-model-name"
              required
              bind:value={controller.state.form.model}
            />
          </div>
        </div>
        <div class="field">
          <label class="label" for="edit-agent-model-parameters"
            >Parameters</label
          >
          <div class="control">
            <input
              class="input"
              id="edit-agent-model-parameters"
              bind:value={controller.state.form.parameters}
            />
          </div>
        </div>
        <div class="field">
          <label class="label" for="edit-agent-model-compaction"
            >Compaction</label
          >
          <div class="control">
            <input
              class="input"
              id="edit-agent-model-compaction"
              bind:value={controller.state.form.compaction}
            />
          </div>
        </div>
        <div class="field">
          <label class="label" for="edit-agent-model-max-turns">Max turns</label
          >
          <div class="control">
            <input
              class="input"
              id="edit-agent-model-max-turns"
              type="number"
              min="0"
              bind:value={controller.state.form.maxTurns}
            />
          </div>
        </div>
        <div class="field">
          <label class="label" for="edit-agent-model-max-output-tokens"
            >Max output tokens</label
          >
          <div class="control">
            <input
              class="input"
              id="edit-agent-model-max-output-tokens"
              type="number"
              min="0"
              bind:value={controller.state.form.maxOutputTokens}
            />
          </div>
        </div>
        <div class="field">
          <label class="checkbox"
            ><input
              type="checkbox"
              bind:checked={controller.state.form.enabled}
            /> Enabled</label
          >
        </div>
        <div class="field is-grouped">
          <p class="control">
            <button
              class="button is-primary"
              type="submit"
              disabled={controller.state.saving}
              >{controller.state.saving ? "Saving..." : "Save changes"}</button
            >
          </p>
          <p class="control">
            <button
              class="button"
              type="button"
              disabled={controller.state.saving}
              onclick={() => (controller.state.editing = false)}>Cancel</button
            >
          </p>
        </div>
      </form>
      {#if controller.state.error !== ""}<p
          class="help is-danger"
          aria-live="polite"
        >
          {controller.state.error}
        </p>{/if}{/if}
  </section></SystemFrame
>
