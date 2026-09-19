<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createWorkspaceAgentBindingController } from "./workspace-agent-binding-controller.svelte"

  let { workspaceID, bindingID }: { workspaceID: string; bindingID: string } =
    $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createWorkspaceAgentBindingController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.load(workspaceID, bindingID)
  })
</script>

<SystemFrame active="workspace-agent-bindings" title="Workspace agent binding">
  <section class="stack">
    {#if controller.state.loading}
      <p class="muted">Loading binding...</p>
    {:else if controller.state.binding === null}
      <p class="field-help" role="alert">
        {controller.state.error || "Workspace agent binding was not found."}
      </p>
      <RouterLink class="secondary" href="/app/system/workspace-agent-bindings"
        >Back to bindings</RouterLink
      >
    {:else if !controller.state.editing}
      <div class="split">
        <div>
          <p class="eyebrow">Workspace agent binding</p>
          <h2>
            {controller.state.binding.workspace} / {controller.state.binding
              .alias}
          </h2>
        </div>
        <button
          class="secondary"
          type="button"
          onclick={() => controller.edit()}>Edit</button
        >
      </div>
      <dl>
        <div class="field">
          <dt>Priority</dt>
          <dd>{controller.state.binding.priority}</dd>
        </div>
        <div class="field">
          <dt>Label</dt>
          <dd>{controller.state.binding.label ?? "Not configured"}</dd>
        </div>
        <div class="field">
          <dt>System prompt</dt>
          <dd>{controller.state.binding.system_prompt ?? "Not configured"}</dd>
        </div>
        <div class="field">
          <dt>Status</dt>
          <dd>{controller.state.binding.enabled ? "Enabled" : "Disabled"}</dd>
        </div>
      </dl>
    {:else}
      <div class="stack">
        <div>
          <p class="eyebrow">Workspace agent binding</p>
          <h2>Edit binding</h2>
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
          <label for="agent-binding-workspace">Workspace ID</label
          >
          <div>
            <input
              id="agent-binding-workspace"
              disabled
              value={controller.state.form.workspace}
            />
          </div>
        </div>
        <div class="field">
          <label for="agent-binding-alias">Alias</label>
          <div>
            <input
              id="agent-binding-alias"
              disabled
              value={controller.state.form.alias}
            />
          </div>
        </div>
        <div class="field">
          <label for="agent-binding-model">Model ID</label>
          <div>
            <input
              id="agent-binding-model"
              bind:value={controller.state.form.model}
            />
          </div>
        </div>
        <div class="field">
          <label for="agent-binding-priority">Priority</label>
          <div>
            <input
              id="agent-binding-priority"
              type="number"
              bind:value={controller.state.form.priority}
            />
          </div>
        </div>
        <div class="field">
          <label for="agent-binding-label">Label</label>
          <div>
            <input
              id="agent-binding-label"
              bind:value={controller.state.form.label}
            />
          </div>
        </div>
        <div class="field">
          <label for="agent-binding-system-prompt"
            >System prompt</label
          >
          <div>
            <textarea
              id="agent-binding-system-prompt"
              rows="4"
              bind:value={controller.state.form.systemPrompt}></textarea>
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
            <button class="primary">Save changes</button>
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
