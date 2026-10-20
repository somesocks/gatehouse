<script lang="ts">
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { untrack } from "svelte"
  import PrincipalPicker from "../principals/PrincipalPicker.svelte"
  import SystemWorkspaceFrame from "./SystemWorkspaceFrame.svelte"
  import { createSystemWorkspacePrincipalNewController } from "./system-workspace-principal-new-controller.svelte"
  let { workspaceID }: { workspaceID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createSystemWorkspacePrincipalNewController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    workspaceID
    return untrack(() => controller.load())
  })
  async function add() {
    const route = runtime.state.route
    if ((await controller.add(workspaceID)) && runtime.state.route === route)
      runtime.navigate(
        `/app/system/workspaces/${encodeURIComponent(workspaceID)}/principals`,
        true,
      )
  }
</script>

<SystemWorkspaceFrame
  {workspaceID}
  breadcrumbs={[
    {
      label: "Principals",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}/principals`,
    },
    { label: "Add" },
  ]}
>
  <UI.AdminContent mode="form"
    ><UI.AdminHeader
      title="Add principal"
      description="Choose an existing principal and grant a direct workspace role."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void add()
      }}
      ><UI.FormSection title="Principal"
        ><PrincipalPicker
          bind:value={controller.state.selectedPrincipalID}
        /></UI.FormSection
      ><UI.FormSection title="Workspace role"
        ><UI.Field
          ><UI.Label for="grant-role">Role</UI.Label><UI.Select
            id="grant-role"
            bind:value={controller.state.role}
            ><UI.Option value="member">Member</UI.Option><UI.Option
              value="contributor">Contributor</UI.Option
            ><UI.Option value="manager">Manager</UI.Option></UI.Select
          ><UI.Text tone="muted" size="small"
            >Adds the selected workspace role. Any existing direct
            roles remain.</UI.Text
          ></UI.Field
        ></UI.FormSection
      >{#if controller.state.error}<UI.Notice variant="destructive" role="alert"
          >{controller.state.error}</UI.Notice
        >{/if}<UI.FormActions
        ><UI.RouterLink
          variant="outline"
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/principals`}
          >Cancel</UI.RouterLink
        ><UI.Button
          type="submit"
          disabled={!controller.state.selectedPrincipalID ||
            controller.state.adding}
          >{controller.state.adding
            ? "Granting…"
            : "Grant workspace access"}</UI.Button
        ></UI.FormActions
      ></UI.Form
    >
  </UI.AdminContent>
</SystemWorkspaceFrame>
