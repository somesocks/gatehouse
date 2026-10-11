<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { untrack } from "svelte"
  import { createSystemWorkspaceFormController } from "./system-workspaces-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createSystemWorkspaceFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  async function create() {
    if (await controller.create())
      runtime.navigate("/app/system/workspaces", true)
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspaces")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspaces", href: "/app/system/workspaces" },
    { label: "Create" },
  ]}
>
  <UI.AdminContent mode="form"
    ><UI.AdminHeader
      title="Create workspace"
      description="Set up workspace identity and availability. Access is assigned separately."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <UI.FormSection title="Identity"
        ><UI.Field
          ><UI.Label for="workspace-alias">Alias</UI.Label><UI.Input
            id="workspace-alias"
            required
            pattern="[a-z][a-z0-9_-]*"
            bind:value={controller.state.alias}
          /><UI.Text size="small" tone="muted"
            >A stable lowercase identifier for this workspace.</UI.Text
          ></UI.Field
        ><UI.Field
          ><UI.Label for="workspace-name">Display name</UI.Label><UI.Input
            id="workspace-name"
            bind:value={controller.state.name}
          /></UI.Field
        ></UI.FormSection
      >
      <UI.FormSection title="Availability"
        ><UI.Label layout="choice"
          ><UI.Checkbox
            bind:checked={controller.state.enabled}
          />Enabled</UI.Label
        ></UI.FormSection
      >
      {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
          >{controller.state.error}</UI.Notice
        >{/if}
      <UI.FormActions
        ><UI.RouterLink variant="outline" href="/app/system/workspaces"
          >Cancel</UI.RouterLink
        ><UI.Button type="submit" disabled={controller.state.saving}
          >{controller.state.saving
            ? "Creating…"
            : "Create workspace"}</UI.Button
        ></UI.FormActions
      >
    </UI.Form>
  </UI.AdminContent>
</UI.AppFrame>
