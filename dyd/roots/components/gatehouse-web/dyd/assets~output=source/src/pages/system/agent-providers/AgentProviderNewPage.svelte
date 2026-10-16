<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { onDestroy, untrack } from "svelte"
  import { createAgentProviderFormController } from "./agent-provider-form-controller.svelte"
  import AgentProviderFields from "./AgentProviderFields.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  onDestroy(controller.stop)
  async function create() {
    const route = runtime.state.route
    const provider = await controller.create()
    if (provider && runtime.state.route === route)
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
    { label: "Agent providers", href: "/app/system/agent-providers" },
    { label: "Create" },
  ]}
>
  <UI.AdminContent mode="form">
    <UI.AdminHeader
      title="Create agent provider"
      description="Configure a service that supplies agent models."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <AgentProviderFields bind:form={controller.state.form} />
      {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
          >{controller.state.error}</UI.Notice
        >{/if}
      <UI.FormActions
        ><UI.RouterLink variant="outline" href="/app/system/agent-providers"
          >Cancel</UI.RouterLink
        ><UI.Button type="submit" disabled={controller.state.saving}
          >{controller.state.saving
            ? "Creating…"
            : "Create provider"}</UI.Button
        ></UI.FormActions
      >
    </UI.Form>
  </UI.AdminContent>
</UI.AppFrame>
