<script lang="ts">
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { untrack } from "svelte"
  import { createWorkspaceAgentBindingsController } from "./workspace-agent-bindings-controller.svelte"
  import WorkspaceAgentBindingFields from "./WorkspaceAgentBindingFields.svelte"
  import SystemWorkspaceFrame from "../workspaces/SystemWorkspaceFrame.svelte"
  let { workspaceID }: { workspaceID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createWorkspaceAgentBindingsController({
      activity: runtime.activity,
      workspaceID,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  async function create() {
    const route = runtime.state.route
    await controller.create()
    if (!controller.state.error && runtime.state.route === route)
      runtime.navigate(
        `/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings`,
        true,
      )
  }
</script>

<SystemWorkspaceFrame
  {workspaceID}
  breadcrumbs={[
    {
      label: "Agent bindings",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings`,
    },
    { label: "Add" },
  ]}
>
  <UI.AdminContent mode="form"
    ><UI.AdminHeader
      title="Add agent binding"
      description="Assign a model and configure its behavior in this workspace."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <WorkspaceAgentBindingFields bind:form={controller.state.form} />
      {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
          >{controller.state.error}</UI.Notice
        >{/if}
      <UI.FormActions
        ><UI.RouterLink
          variant="outline"
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings`}
          >Cancel</UI.RouterLink
        ><UI.Button
          type="submit"
          disabled={controller.state.saving || !controller.state.form.model}
          >{controller.state.saving
            ? "Adding…"
            : "Add agent binding"}</UI.Button
        ></UI.FormActions
      >
    </UI.Form>
  </UI.AdminContent>
</SystemWorkspaceFrame>
