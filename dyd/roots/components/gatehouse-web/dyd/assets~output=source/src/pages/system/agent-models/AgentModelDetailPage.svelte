<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

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

<UI.AppFrame
  navigation={systemNavigation(runtime, "agent-models")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Agent model" },
  ]}
  ><UI.Stack as="section">
    {#if controller.state.loading}<UI.Text as="p" tone="muted">
        Loading agent model...
      </UI.Text>{:else if controller.state.model === null}<UI.Text
        as="p"
        size="small"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error === ""
          ? "Agent model was not found."
          : controller.state.error}
      </UI.Text>
      <RouterLink variant="outline" href="/app/system/agent-models"
        >Back to models</RouterLink
      >{:else if !controller.state.editing}<UI.Row justify="between">
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Agent model</UI.Text>
          <UI.Heading level={2}>{controller.state.model.alias}</UI.Heading>
          <UI.Text as="p">{controller.state.model.id}</UI.Text>
        </UI.Group>
        <UI.Button
          variant="outline"
          type="button"
          onclick={() => void controller.beginEdit()}>Edit</UI.Button
        >
      </UI.Row>
      <UI.Stack as="dl">
        <UI.Field>
          <UI.Text as="dt" size="small">Provider</UI.Text>
          <UI.Text as="dd">{controller.state.model.provider}</UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Model</UI.Text>
          <UI.Text as="dd">{controller.state.model.model}</UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Parameters</UI.Text>
          <UI.Text as="dd">
            {controller.state.model.parameters === ""
              ? "Not configured"
              : controller.state.model.parameters}
          </UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Compaction</UI.Text>
          <UI.Text as="dd">
            {controller.state.model.compaction === ""
              ? "Not configured"
              : controller.state.model.compaction}
          </UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Max turns</UI.Text>
          <UI.Text as="dd">{controller.state.model.max_turns}</UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Max output tokens</UI.Text>
          <UI.Text as="dd">{controller.state.model.max_output_tokens}</UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Status</UI.Text>
          <UI.Text as="dd">
            {controller.state.model.enabled ? "Enabled" : "Disabled"}
          </UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Revision</UI.Text>
          <UI.Text as="dd">{controller.state.model.revision}</UI.Text>
        </UI.Field>
      </UI.Stack>{:else}<UI.Stack>
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Agent model</UI.Text>
          <UI.Heading level={2}>Edit {controller.state.model.alias}</UI.Heading>
          <UI.Text as="p">Update model settings.</UI.Text>
        </UI.Group>
      </UI.Stack>
      <UI.Form
        onsubmit={(event) => {
          event.preventDefault()
          void controller.update()
        }}
      >
        <UI.Field>
          <UI.Label for="edit-agent-model-alias">Alias</UI.Label>
          <UI.Group>
            <UI.Input
              id="edit-agent-model-alias"
              disabled
              value={controller.state.form.alias}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="edit-agent-model-provider">Provider</UI.Label>
          <UI.Group>
            <UI.Group>
              <UI.Select
                id="edit-agent-model-provider"
                required
                bind:value={controller.state.form.provider}
                ><UI.Option value="">Select provider</UI.Option
                >{#each controller.state.providers as provider (provider.id)}<UI.Option
                    value={provider.id}
                    disabled={!provider.enabled}
                    >{provider.alias} / {provider.id}{provider.enabled
                      ? ""
                      : " / Disabled"}</UI.Option
                  >{/each}</UI.Select
              >
            </UI.Group>
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="edit-agent-model-name">Model</UI.Label>
          <UI.Group>
            <UI.Input
              id="edit-agent-model-name"
              required
              bind:value={controller.state.form.model}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="edit-agent-model-parameters">Parameters</UI.Label>
          <UI.Group>
            <UI.Input
              id="edit-agent-model-parameters"
              bind:value={controller.state.form.parameters}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="edit-agent-model-compaction">Compaction</UI.Label>
          <UI.Group>
            <UI.Input
              id="edit-agent-model-compaction"
              bind:value={controller.state.form.compaction}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="edit-agent-model-max-turns">Max turns</UI.Label>
          <UI.Group>
            <UI.Input
              id="edit-agent-model-max-turns"
              type="number"
              min="0"
              bind:value={controller.state.form.maxTurns}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="edit-agent-model-max-output-tokens"
            >Max output tokens</UI.Label
          >
          <UI.Group>
            <UI.Input
              id="edit-agent-model-max-output-tokens"
              type="number"
              min="0"
              bind:value={controller.state.form.maxOutputTokens}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label layout="choice"
            ><UI.Checkbox bind:checked={controller.state.form.enabled} /> Enabled</UI.Label
          >
        </UI.Field>
        <UI.Row>
          <UI.Group>
            <UI.Button
              variant="default"
              type="submit"
              disabled={controller.state.saving}
              >{controller.state.saving
                ? "Saving..."
                : "Save changes"}</UI.Button
            >
          </UI.Group>
          <UI.Group>
            <UI.Button
              variant="outline"
              type="button"
              disabled={controller.state.saving}
              onclick={() => (controller.state.editing = false)}
              >Cancel</UI.Button
            >
          </UI.Group>
        </UI.Row>
      </UI.Form>
      {#if controller.state.error !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </UI.Text>{/if}{/if}
  </UI.Stack></UI.AppFrame
>
