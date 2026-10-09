<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createAgentProviderFormController } from "./agent-provider-form-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.keychains()
  })
  async function create(): Promise<void> {
    const provider = await controller.create()
    if (provider !== null)
      runtime.navigate(
        `/app/system/agent-providers/${encodeURIComponent(provider.id)}`,
        true,
      )
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "agent-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "New agent provider" },
  ]}
>
  <UI.Stack as="section">
    <UI.Stack>
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>New agent provider</UI.Heading>
        <UI.Text as="p">
          Credentials are write-only and cannot be viewed after saving.
        </UI.Text>
      </UI.Group>
    </UI.Stack>
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <UI.Label layout="field"
        ><UI.Text>Alias</UI.Text><UI.Input
          required
          bind:value={controller.state.form.alias}
        /></UI.Label
      ><UI.Label layout="field"
        ><UI.Text>Protocol</UI.Text><UI.Select
          bind:value={controller.state.form.protocol}
          ><UI.Option value="builtin">Built-in</UI.Option><UI.Option
            value="openai-chat-completions">OpenAI chat completions</UI.Option
          ><UI.Option value="openai-responses">OpenAI responses</UI.Option
          ></UI.Select
        ></UI.Label
      ><UI.Label layout="field"
        ><UI.Text>Base URL</UI.Text><UI.Input
          bind:value={controller.state.form.baseURL}
        /></UI.Label
      >{#if controller.state.form.protocol !== "builtin"}<UI.Label
          layout="field"
          ><UI.Text>Keychain</UI.Text><UI.Select
            required
            bind:value={controller.state.form.keychain}
            ><UI.Option value="">Select keychain</UI.Option
            >{#each controller.state.keychains as keychain (`${keychain.id}/${keychain.version}`)}<UI.Option
                value={keychain.id}
                >{keychain.id} / version {keychain.version}</UI.Option
              >{/each}</UI.Select
          ></UI.Label
        ><UI.Label layout="field"
          ><UI.Text>API key</UI.Text><UI.Input
            type="password"
            autocomplete="new-password"
            required
            bind:value={controller.state.form.apiKey}
          /></UI.Label
        >{/if}<UI.Button
        variant="default"
        type="submit"
        disabled={controller.state.saving}
        >{controller.state.saving ? "Saving..." : "Add provider"}</UI.Button
      ><RouterLink variant="outline" href="/app/system/agent-providers"
        >Cancel</RouterLink
      >
    </UI.Form>
    {#if controller.state.error !== ""}<UI.Text
        as="p"
        size="small"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error}
      </UI.Text>{/if}
  </UI.Stack>
</UI.AppFrame>
