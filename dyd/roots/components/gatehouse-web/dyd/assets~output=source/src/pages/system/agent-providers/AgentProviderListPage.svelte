<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createAgentProviderListController } from "./agent-provider-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentProviderListController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  const matches = (
    provider: { id: string; alias: string; protocol: string },
    search: string,
  ) => {
    const query = search.trim().toLocaleLowerCase()
    return (
      query === "" ||
      `${provider.id} ${provider.alias} ${provider.protocol}`
        .toLocaleLowerCase()
        .includes(query)
    )
  }
  $effect(() => {
    void controller.load()
  })
  $effect(() => controller.start())
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "agent-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Agent providers" },
  ]}
>
  <UI.Stack as="section">
    <UI.Row justify="between">
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>Agent providers</UI.Heading>
        <UI.Text as="p">
          Configure model-provider protocols and credentials.
        </UI.Text>
      </UI.Group>
      <RouterLink variant="default" href="/app/system/agent-providers/new"
        >Add provider</RouterLink
      >
    </UI.Row>
    <UI.Field>
      <UI.Label
        ><UI.Text>Search agent providers</UI.Text><UI.Input
          type="search"
          autocomplete="off"
          placeholder="Search by ID, alias, or protocol"
          bind:value={controller.state.search}
        /></UI.Label
      >
    </UI.Field>
    {#if controller.state.error !== ""}<UI.Text
        as="p"
        size="small"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error}
      </UI.Text>{/if}
    <UI.List>
      {#each controller.state.providers.filter( (provider) => matches(provider, controller.state.search), ) as provider (provider.id)}<RouterLink
          variant="item"
          data-disabled={!provider.enabled || undefined}
          href={`/app/system/agent-providers/${encodeURIComponent(provider.id)}`}
          ><UI.Stack>
            <UI.Text as="strong" emphasis="strong">{provider.alias}</UI.Text
            ><UI.Text as="small" size="small"
              >{provider.id} / {provider.protocol} / revision {provider.revision}{provider.credential_configured
                ? " / credential configured"
                : ""}</UI.Text
            >
          </UI.Stack>
          <UI.Text>{provider.enabled ? "Enabled" : "Disabled"}</UI.Text
          ></RouterLink
        >{:else}<UI.Text as="p" tone="muted">
          No agent providers match your search.
        </UI.Text>{/each}
    </UI.List>
  </UI.Stack>
</UI.AppFrame>
