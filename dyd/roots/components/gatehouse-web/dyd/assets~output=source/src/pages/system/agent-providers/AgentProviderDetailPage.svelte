<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createAgentProviderFormController } from "./agent-provider-form-controller.svelte"

  let { providerID }: { providerID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.load(providerID)
  })
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "agent-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Agent provider" },
  ]}
>
  <UI.Stack as="section">
    {#if controller.state.loading}
      <UI.Text as="p" tone="muted">Loading agent provider...</UI.Text>
    {:else if controller.state.provider === null}
      <UI.Text as="p" size="small" role="alert" aria-live="polite">
        {controller.state.error === ""
          ? "Agent provider was not found."
          : controller.state.error}
      </UI.Text>
      <RouterLink variant="outline" href="/app/system/agent-providers"
        >Back to providers</RouterLink
      >
    {:else if !controller.state.editing}
      <UI.Row justify="between">
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Agent provider</UI.Text>
          <UI.Heading level={2}>{controller.state.provider.alias}</UI.Heading>
          <UI.Text as="p">{controller.state.provider.id}</UI.Text>
        </UI.Group>
        <UI.Button
          variant="outline"
          type="button"
          onclick={() => void controller.beginEdit()}>Edit</UI.Button
        >
      </UI.Row>
      <UI.Stack as="dl">
        <UI.Field>
          <UI.Text as="dt" size="small">Protocol</UI.Text>
          <UI.Text as="dd">{controller.state.provider.protocol}</UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Base URL</UI.Text>
          <UI.Text as="dd">
            {controller.state.provider.base_url ?? "Not configured"}
          </UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Keychain</UI.Text>
          <UI.Text as="dd">
            {controller.state.provider.keychain === undefined
              ? "Not configured"
              : `${controller.state.provider.keychain.id} / version ${controller.state.provider.keychain.version}`}
          </UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Credential</UI.Text>
          <UI.Text as="dd">
            {controller.state.provider.credential_configured
              ? "Configured"
              : "Not configured"}
          </UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Status</UI.Text>
          <UI.Text as="dd">
            {controller.state.provider.enabled ? "Enabled" : "Disabled"}
          </UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Revision</UI.Text>
          <UI.Text as="dd">{controller.state.provider.revision}</UI.Text>
        </UI.Field>
      </UI.Stack>
    {:else}
      <UI.Stack>
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Agent provider</UI.Text>
          <UI.Heading level={2}
            >Edit {controller.state.provider.alias}</UI.Heading
          >
          <UI.Text as="p">Update provider settings.</UI.Text>
        </UI.Group>
      </UI.Stack>
      <UI.Form
        onsubmit={(event) => {
          event.preventDefault()
          void controller.update()
        }}
      >
        <UI.Field>
          <UI.Label for="agent-provider-alias">Alias</UI.Label>
          <UI.Group>
            <UI.Input
              id="agent-provider-alias"
              disabled
              value={controller.state.form.alias}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="agent-provider-protocol">Protocol</UI.Label>
          <UI.Group>
            <UI.Group>
              <UI.Select
                id="agent-provider-protocol"
                bind:value={controller.state.form.protocol}
                ><UI.Option value="builtin">Built-in</UI.Option><UI.Option
                  value="openai-chat-completions"
                  >OpenAI chat completions</UI.Option
                ><UI.Option value="openai-responses">OpenAI responses</UI.Option
                ></UI.Select
              >
            </UI.Group>
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="agent-provider-base-url">Base URL</UI.Label>
          <UI.Group>
            <UI.Input
              id="agent-provider-base-url"
              bind:value={controller.state.form.baseURL}
            />
          </UI.Group>
        </UI.Field>
        {#if controller.state.form.protocol !== "builtin"}
          <UI.Field>
            <UI.Label for="agent-provider-keychain">Keychain</UI.Label>
            <UI.Group>
              <UI.Group>
                <UI.Select
                  id="agent-provider-keychain"
                  required
                  bind:value={controller.state.form.keychain}
                  ><UI.Option value="">Select keychain</UI.Option
                  >{#each controller.state.keychains as keychain (`${keychain.id}/${keychain.version}`)}<UI.Option
                      value={keychain.id}
                      >{keychain.id} / version {keychain.version}</UI.Option
                    >{/each}</UI.Select
                >
              </UI.Group>
            </UI.Group>
          </UI.Field>
          <UI.Field>
            <UI.Label for="agent-provider-api-key">Replacement API key</UI.Label
            >
            <UI.Group>
              <UI.Input
                id="agent-provider-api-key"
                type="password"
                autocomplete="new-password"
                bind:value={controller.state.form.apiKey}
              />
            </UI.Group>
            <UI.Text as="p" size="small">
              Leave blank to preserve the current API key. Required when
              changing protocol or keychain.
            </UI.Text>
          </UI.Field>
        {/if}
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
        </UI.Text>{/if}
    {/if}
  </UI.Stack>
</UI.AppFrame>
