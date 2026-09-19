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
  ><section class="stack">
    {#if controller.state.loading}<p class="muted">
        Loading agent model...
      </p>{:else if controller.state.model === null}<p
        class="field-help"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error === ""
          ? "Agent model was not found."
          : controller.state.error}
      </p>
      <RouterLink class="secondary" href="/app/system/agent-models"
        >Back to models</RouterLink
      >{:else if !controller.state.editing}<div
        class="split"
      >
        <div>
          <p class="eyebrow">Agent model</p>
          <h2>{controller.state.model.alias}</h2>
          <p>{controller.state.model.id}</p>
        </div>
        <button
          class="secondary"
          type="button"
          onclick={() => void controller.beginEdit()}>Edit</button
        >
      </div>
      <dl>
        <div class="field">
          <dt>Provider</dt>
          <dd>{controller.state.model.provider}</dd>
        </div>
        <div class="field">
          <dt>Model</dt>
          <dd>{controller.state.model.model}</dd>
        </div>
        <div class="field">
          <dt>Parameters</dt>
          <dd>
            {controller.state.model.parameters === ""
              ? "Not configured"
              : controller.state.model.parameters}
          </dd>
        </div>
        <div class="field">
          <dt>Compaction</dt>
          <dd>
            {controller.state.model.compaction === ""
              ? "Not configured"
              : controller.state.model.compaction}
          </dd>
        </div>
        <div class="field">
          <dt>Max turns</dt>
          <dd>{controller.state.model.max_turns}</dd>
        </div>
        <div class="field">
          <dt>Max output tokens</dt>
          <dd>{controller.state.model.max_output_tokens}</dd>
        </div>
        <div class="field">
          <dt>Status</dt>
          <dd>
            {controller.state.model.enabled ? "Enabled" : "Disabled"}
          </dd>
        </div>
        <div class="field">
          <dt>Revision</dt>
          <dd>{controller.state.model.revision}</dd>
        </div>
      </dl>{:else}<div class="stack">
        <div>
          <p class="eyebrow">Agent model</p>
          <h2>Edit {controller.state.model.alias}</h2>
          <p>Update model settings.</p>
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
          <label for="edit-agent-model-alias">Alias</label>
          <div>
            <input
              id="edit-agent-model-alias"
              disabled
              value={controller.state.form.alias}
            />
          </div>
        </div>
        <div class="field">
          <label for="edit-agent-model-provider">Provider</label>
          <div>
            <div>
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
          <label for="edit-agent-model-name">Model</label>
          <div>
            <input
              id="edit-agent-model-name"
              required
              bind:value={controller.state.form.model}
            />
          </div>
        </div>
        <div class="field">
          <label for="edit-agent-model-parameters"
            >Parameters</label
          >
          <div>
            <input
              id="edit-agent-model-parameters"
              bind:value={controller.state.form.parameters}
            />
          </div>
        </div>
        <div class="field">
          <label for="edit-agent-model-compaction"
            >Compaction</label
          >
          <div>
            <input
              id="edit-agent-model-compaction"
              bind:value={controller.state.form.compaction}
            />
          </div>
        </div>
        <div class="field">
          <label for="edit-agent-model-max-turns">Max turns</label
          >
          <div>
            <input
              id="edit-agent-model-max-turns"
              type="number"
              min="0"
              bind:value={controller.state.form.maxTurns}
            />
          </div>
        </div>
        <div class="field">
          <label for="edit-agent-model-max-output-tokens"
            >Max output tokens</label
          >
          <div>
            <input
              id="edit-agent-model-max-output-tokens"
              type="number"
              min="0"
              bind:value={controller.state.form.maxOutputTokens}
            />
          </div>
        </div>
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
        </p>{/if}{/if}
  </section></SystemFrame
>
