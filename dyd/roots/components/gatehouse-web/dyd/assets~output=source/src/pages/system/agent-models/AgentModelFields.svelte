<script lang="ts">
  import * as UI from "../../../ui"
  import type { createAgentModelFormController } from "./agent-model-form-controller.svelte"
  import AgentProviderPicker from "../agent-providers/AgentProviderPicker.svelte"
  let {
    form = $bindable(),
    editing = false,
  }: {
    form: ReturnType<typeof createAgentModelFormController>["state"]["form"]
    editing?: boolean
  } = $props()
</script>

<UI.FormSection title="Model selection">
  {#if !editing}<UI.Field
      ><UI.Label for="model-alias">Alias</UI.Label><UI.Input
        id="model-alias"
        required
        pattern="[a-z][a-z0-9_-]*"
        bind:value={form.alias}
      /></UI.Field
    >{/if}
  <AgentProviderPicker bind:value={form.provider} />
  <UI.Field span="full"
    ><UI.Label for="model-name">Model</UI.Label><UI.Input
      id="model-name"
      required
      bind:value={form.model}
    /><UI.Text as="small" size="small" tone="muted"
      >The provider’s model identifier.</UI.Text
    ></UI.Field
  >
</UI.FormSection>
<UI.FormSection title="Generation limits"
  ><UI.Grid columns={2}>
    <UI.Field
      ><UI.Label for="model-turns">Maximum turns</UI.Label><UI.Input
        id="model-turns"
        type="number"
        min="1"
        required
        bind:value={form.maxTurns}
      /></UI.Field
    >
    <UI.Field
      ><UI.Label for="model-tokens">Maximum output tokens</UI.Label><UI.Input
        id="model-tokens"
        type="number"
        min="1"
        required
        bind:value={form.maxOutputTokens}
      /></UI.Field
    >
  </UI.Grid></UI.FormSection
>
<UI.FormSection
  title="Advanced configuration"
  description="Provider parameters and context compaction are configured as JSON."
>
  <UI.Field
    ><UI.Label for="model-parameters">Parameters</UI.Label><UI.JsonInput
      id="model-parameters"
      required
      bind:value={form.parameters}
    /></UI.Field
  >
  <UI.Field
    ><UI.Label for="model-compaction">Compaction</UI.Label><UI.JsonInput
      id="model-compaction"
      required
      bind:value={form.compaction}
    /></UI.Field
  >
</UI.FormSection>
<UI.FormSection title="Availability"
  ><UI.Label layout="choice"
    ><UI.Checkbox bind:checked={form.enabled} />Enabled</UI.Label
  ></UI.FormSection
>
