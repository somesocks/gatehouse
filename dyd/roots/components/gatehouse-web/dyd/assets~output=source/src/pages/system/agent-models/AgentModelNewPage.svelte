<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

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

<UI.AppFrame
  navigation={systemNavigation(runtime, "agent-models")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "New agent model" },
  ]}
  ><UI.Stack as="section">
    <UI.Stack>
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>New agent model</UI.Heading>
        <UI.Text as="p">
          Configure a model available for workspace bindings.
        </UI.Text>
      </UI.Group>
    </UI.Stack>
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <UI.Field>
        <UI.Label for="agent-model-alias">Alias</UI.Label>
        <UI.Group>
          <UI.Input
            id="agent-model-alias"
            required
            bind:value={controller.state.form.alias}
          />
        </UI.Group>
      </UI.Field>
      <UI.Field>
        <UI.Label for="agent-model-provider">Provider</UI.Label>
        <UI.Group>
          <UI.Group>
            <UI.Select
              id="agent-model-provider"
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
        <UI.Label for="agent-model-name">Model</UI.Label>
        <UI.Group>
          <UI.Input
            id="agent-model-name"
            required
            bind:value={controller.state.form.model}
          />
        </UI.Group>
      </UI.Field>
      <UI.Field>
        <UI.Label for="agent-model-parameters">Parameters</UI.Label>
        <UI.Group>
          <UI.Input
            id="agent-model-parameters"
            bind:value={controller.state.form.parameters}
          />
        </UI.Group>
      </UI.Field>
      <UI.Field>
        <UI.Label for="agent-model-compaction">Compaction</UI.Label>
        <UI.Group>
          <UI.Input
            id="agent-model-compaction"
            bind:value={controller.state.form.compaction}
          />
        </UI.Group>
      </UI.Field>
      <UI.Field>
        <UI.Label for="agent-model-max-turns">Max turns</UI.Label>
        <UI.Group>
          <UI.Input
            id="agent-model-max-turns"
            type="number"
            min="0"
            bind:value={controller.state.form.maxTurns}
          />
        </UI.Group>
      </UI.Field>
      <UI.Field>
        <UI.Label for="agent-model-max-output-tokens"
          >Max output tokens</UI.Label
        >
        <UI.Group>
          <UI.Input
            id="agent-model-max-output-tokens"
            type="number"
            min="0"
            bind:value={controller.state.form.maxOutputTokens}
          />
        </UI.Group>
      </UI.Field>
      <UI.Row>
        <UI.Group>
          <UI.Button
            variant="default"
            type="submit"
            disabled={controller.state.saving}
            >{controller.state.saving ? "Saving..." : "Add model"}</UI.Button
          >
        </UI.Group>
        <UI.Group>
          <RouterLink variant="outline" href="/app/system/agent-models"
            >Cancel</RouterLink
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
      </UI.Text>{/if}
  </UI.Stack></UI.AppFrame
>
