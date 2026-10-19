<script lang="ts">
  import * as UI from "../../../ui"
  import AgentModelPicker from "../agent-models/AgentModelPicker.svelte"
  let {
    form = $bindable(),
    editing = false,
  }: {
    form: {
      alias: string
      model: string
      label: string
      systemPrompt: string
      prelude: string
      rateLimits: string
      default: boolean
      enabled?: boolean
    }
    editing?: boolean
  } = $props()
</script>

<UI.FormSection title="Model assignment">
  {#if !editing}<UI.Field
      ><UI.Label for="binding-alias">Binding alias</UI.Label><UI.Input
        id="binding-alias"
        required
        bind:value={form.alias}
      /><UI.Text as="small" size="small" tone="muted"
        >A stable alias used to address this workspace agent.</UI.Text
      ></UI.Field
    >{/if}
  <AgentModelPicker bind:value={form.model} />
</UI.FormSection>
<UI.FormSection title="Display"
  ><UI.Field
    ><UI.Label for="binding-label">Label</UI.Label><UI.Input
      id="binding-label"
      bind:value={form.label}
    /></UI.Field
  ></UI.FormSection
>
<UI.FormSection title="Prompt and prelude">
  <UI.Field
    ><UI.Label for="binding-prompt">Custom system prompt</UI.Label><UI.Textarea
      id="binding-prompt"
      rows={5}
      bind:value={form.systemPrompt}
    /></UI.Field
  >
  <UI.Field
    ><UI.Label for="binding-prelude">Lisp prelude</UI.Label><UI.Textarea
      id="binding-prelude"
      rows={6}
      bind:value={form.prelude}
    /></UI.Field
  >
</UI.FormSection>
<UI.FormSection
  title="Token rate limits"
  description="Configure limits independently for workspace and user input/output."
  ><UI.RateLimitsEditor bind:value={form.rateLimits} /></UI.FormSection
>
<UI.FormSection title="Availability"
  ><UI.Label layout="choice"
    ><UI.Checkbox bind:checked={form.default} />Available to all workspace
    members</UI.Label
  >{#if form.enabled !== undefined}<UI.Label layout="choice"
      ><UI.Checkbox bind:checked={form.enabled} />Enabled</UI.Label
    >{/if}</UI.FormSection
>
