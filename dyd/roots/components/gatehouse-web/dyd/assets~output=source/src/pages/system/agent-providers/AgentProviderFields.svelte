<script lang="ts">
  import * as UI from "../../../ui"
  import type { createAgentProviderFormController } from "./agent-provider-form-controller.svelte"
  import type { SystemKeychain } from "../../../app/system"
  import KeychainPicker from "../keychains/KeychainPicker.svelte"
  let {
    form = $bindable(),
    keychains,
    keychainsLoading = false,
    keychainsError = "",
    onKeychainsReload,
    editing = false,
  }: {
    form: ReturnType<typeof createAgentProviderFormController>["state"]["form"]
    keychains: SystemKeychain[]
    keychainsLoading?: boolean
    keychainsError?: string
    onKeychainsReload?: () => void
    editing?: boolean
  } = $props()
</script>

{#if !editing}<UI.FormSection title="Identity">
    <UI.Field
      ><UI.Label for="provider-alias">Alias</UI.Label><UI.Input
        id="provider-alias"
        required
        pattern="[a-z][a-z0-9_-]*"
        bind:value={form.alias}
      /><UI.Text as="small" size="small" tone="muted"
        >A stable lowercase identifier for this provider.</UI.Text
      ></UI.Field
    >
  </UI.FormSection>{/if}
<UI.FormSection title="Connection">
  <UI.Field
    ><UI.Label for="provider-protocol">Protocol</UI.Label><UI.Select
      id="provider-protocol"
      bind:value={form.protocol}
      ><UI.Option value="builtin">Built-in</UI.Option><UI.Option
        value="openai-responses">OpenAI Responses</UI.Option
      ><UI.Option value="openai-chat-completions"
        >OpenAI Chat Completions</UI.Option
      ></UI.Select
    ></UI.Field
  >
  {#if form.protocol !== "builtin"}<UI.Field
      ><UI.Label for="provider-url">Base URL</UI.Label><UI.Input
        id="provider-url"
        type="url"
        bind:value={form.baseURL}
        placeholder="https://api.example.com/v1"
      /></UI.Field
    >{/if}
</UI.FormSection>
{#if form.protocol !== "builtin"}<UI.FormSection
    title="Credentials"
    description="Credentials are write-only. Existing secrets are never displayed."
  >
    <KeychainPicker
      bind:value={form.keychain}
      {keychains}
      loading={keychainsLoading}
      error={keychainsError}
      onreload={onKeychainsReload}
    />
    <UI.Field
      ><UI.Label for="provider-key"
        >{editing ? "Replacement API key" : "API key"}</UI.Label
      ><UI.Input
        id="provider-key"
        type="password"
        autocomplete="new-password"
        required={!editing}
        bind:value={form.apiKey}
      />{#if editing}<UI.Text as="small" size="small" tone="muted"
          >Leave blank to keep the current key. Replace it when changing
          protocol or keychain.</UI.Text
        >{/if}</UI.Field
    >
  </UI.FormSection>{/if}
<UI.FormSection title="Availability"
  ><UI.Label layout="choice"
    ><UI.Checkbox bind:checked={form.enabled} />Enabled</UI.Label
  ></UI.FormSection
>
