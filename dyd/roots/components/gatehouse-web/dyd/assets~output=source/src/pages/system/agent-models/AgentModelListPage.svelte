<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createAgentModelListController } from "./agent-model-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentModelListController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  const matches = (
    model: { id: string; alias: string; provider: string; model: string },
    search: string,
  ) => {
    const query = search.trim().toLocaleLowerCase()
    return (
      query === "" ||
      `${model.id} ${model.alias} ${model.provider} ${model.model}`
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
  navigation={systemNavigation(runtime, "agent-models")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Agent models" },
  ]}
  ><UI.Stack as="section">
    <UI.Row justify="between">
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>Agent models</UI.Heading>
        <UI.Text as="p">
          Workspace bindings designate the model used for automatic replies.
        </UI.Text>
      </UI.Group>
      <RouterLink variant="default" href="/app/system/agent-models/new"
        >Add model</RouterLink
      >
    </UI.Row>
    <UI.Field>
      <UI.Label
        ><UI.Text>Search agent models</UI.Text><UI.Input
          type="search"
          autocomplete="off"
          placeholder="Search by ID, alias, provider, or model"
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
      {#each controller.state.models.filter( (model) => matches(model, controller.state.search), ) as model (model.id)}<RouterLink
          variant="item"
          data-disabled={!model.enabled || undefined}
          href={`/app/system/agent-models/${encodeURIComponent(model.id)}`}
          ><UI.Stack>
            <UI.Text as="strong" emphasis="strong">{model.alias}</UI.Text
            ><UI.Text as="small" size="small"
              >{model.id} / {model.provider} / {model.model} / revision {model.revision}</UI.Text
            >
          </UI.Stack>
          <UI.Text>{model.enabled ? "Enabled" : "Disabled"}</UI.Text
          ></RouterLink
        >{:else}<UI.Text as="p" tone="muted">
          No agent models match your search.
        </UI.Text>{/each}
    </UI.List>
  </UI.Stack></UI.AppFrame
>
