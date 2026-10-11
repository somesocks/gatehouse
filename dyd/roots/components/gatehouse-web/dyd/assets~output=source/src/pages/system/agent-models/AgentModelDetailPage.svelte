<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { untrack } from "svelte"
  import { createAgentModelFormController } from "./agent-model-form-controller.svelte"
  import AgentModelFields from "./AgentModelFields.svelte"
  import {
    systemAdministration,
    type SystemAgentProvider,
  } from "../../../app/system"
  let {
    modelID,
    editingPage = false,
  }: { modelID: string; editingPage?: boolean } = $props()
  const runtime = useRuntime()
  let provider = $state<SystemAgentProvider | null>(null)
  const controller = untrack(() =>
    createAgentModelFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    const id = modelID
    const edit = editingPage
    void controller.load(id).then(() => {
      if (edit) void controller.beginEdit()
    })
  })
  $effect(() => {
    const id = controller.state.model?.provider
    provider = null
    if (!id) return
    const request = new AbortController()
    void systemAdministration(
      `agent-providers/${encodeURIComponent(id)}`,
      "GET",
      undefined,
      request.signal,
    )
      .then(async (response) => {
        if (response.ok) {
          const result = (await response.json()) as SystemAgentProvider
          if (!request.signal.aborted) provider = result
        }
      })
      .catch(() => {})
    return () => request.abort()
  })
  async function save() {
    const route = runtime.state.route
    if ((await controller.update()) && runtime.state.route === route)
      runtime.navigate(
        `/app/system/agent-models/${encodeURIComponent(modelID)}`,
        true,
      )
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "agent-models")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Agent models", href: "/app/system/agent-models" },
    {
      label: controller.state.model?.alias ?? "Model",
      href: editingPage
        ? `/app/system/agent-models/${encodeURIComponent(modelID)}`
        : undefined,
    },
    ...(editingPage ? [{ label: "Edit" }] : []),
  ]}
>
  <UI.AdminContent mode={editingPage ? "form" : "detail"}>
    {#if controller.state.loading}<UI.Text tone="muted">Loading model…</UI.Text>
    {:else if !controller.state.model}<UI.Notice
        variant="destructive"
        role="alert"
        >{controller.state.error || "Model was not found."}</UI.Notice
      >
    {:else if editingPage}
      <UI.AdminHeader
        title={`Edit ${controller.state.model.alias}`}
        id={modelID}
        description="Update model selection, generation limits, and availability."
      />
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <AgentModelFields bind:form={controller.state.form} editing />
        {#if controller.state.error}<UI.Notice
            variant="destructive"
            role="alert">{controller.state.error}</UI.Notice
          >{/if}
        <UI.FormActions
          ><UI.RouterLink
            variant="outline"
            href={`/app/system/agent-models/${encodeURIComponent(modelID)}`}
            >Cancel</UI.RouterLink
          ><UI.Button type="submit" disabled={controller.state.saving}
            >{controller.state.saving ? "Saving…" : "Save changes"}</UI.Button
          ></UI.FormActions
        >
      </UI.Form>
    {:else}
      <UI.AdminHeader
        kind="Agent model"
        title={controller.state.model.alias}
        id={modelID}
        enabled={controller.state.model.enabled}
        >{#snippet actions()}<UI.RouterLink
            variant="outline"
            href={`/app/system/agent-models/${encodeURIComponent(modelID)}/edit`}
            >Edit model</UI.RouterLink
          >{/snippet}</UI.AdminHeader
      >
      <UI.DetailSection title="Model selection"
        ><UI.Property label="Provider"
          ><UI.RouterLink
            href={`/app/system/agent-providers/${encodeURIComponent(controller.state.model.provider)}`}
            >{provider?.alias ?? controller.state.model.provider}</UI.RouterLink
          ></UI.Property
        ><UI.Property label="Model">{controller.state.model.model}</UI.Property
        ></UI.DetailSection
      >
      <UI.DetailSection title="Generation limits"
        ><UI.Property label="Maximum turns"
          >{controller.state.model.max_turns}</UI.Property
        ><UI.Property label="Output tokens"
          >{controller.state.model.max_output_tokens}</UI.Property
        ></UI.DetailSection
      >
      <UI.DetailSection title="Advanced configuration"
        ><UI.Property label="Parameters"
          ><UI.CodeBlock
            value={controller.state.model.parameters}
            json
          /></UI.Property
        ><UI.Property label="Compaction"
          ><UI.CodeBlock
            value={controller.state.model.compaction}
            json
          /></UI.Property
        ></UI.DetailSection
      >
      <UI.DetailSection title="Record information"
        ><UI.Property label="Alias">{controller.state.model.alias}</UI.Property
        ><UI.Property label="Revision"
          >{controller.state.model.revision}</UI.Property
        ></UI.DetailSection
      >
    {/if}
  </UI.AdminContent>
</UI.AppFrame>
