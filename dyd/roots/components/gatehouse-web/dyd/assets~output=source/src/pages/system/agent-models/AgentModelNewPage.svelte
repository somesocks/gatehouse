<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { untrack } from "svelte"
  import { createAgentModelFormController } from "./agent-model-form-controller.svelte"
  import AgentModelFields from "./AgentModelFields.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentModelFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  async function create() {
    const route = runtime.state.route
    const model = await controller.create()
    if (model && runtime.state.route === route)
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
    { label: "Agent models", href: "/app/system/agent-models" },
    { label: "Create" },
  ]}
>
  <UI.AdminContent mode="form"
    ><UI.AdminHeader
      title="Create agent model"
      description="Choose a provider and configure model generation settings."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <AgentModelFields bind:form={controller.state.form} />
      {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
          >{controller.state.error}</UI.Notice
        >{/if}
      <UI.FormActions
        ><UI.RouterLink variant="outline" href="/app/system/agent-models"
          >Cancel</UI.RouterLink
        ><UI.Button type="submit" disabled={controller.state.saving}
          >{controller.state.saving ? "Creating…" : "Create model"}</UI.Button
        ></UI.FormActions
      >
    </UI.Form>
  </UI.AdminContent>
</UI.AppFrame>
