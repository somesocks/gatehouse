<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { untrack } from "svelte"
  import { createSystemWorkspaceDetailController } from "./system-workspace-detail-controller.svelte"
  let { workspaceID }: { workspaceID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createSystemWorkspaceDetailController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    const id = workspaceID
    return untrack(() => controller.load(id))
  })
  async function save() {
    if (await controller.save(workspaceID))
      runtime.navigate(
        `/app/system/workspaces/${encodeURIComponent(workspaceID)}`,
        true,
      )
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspaces")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspaces", href: "/app/system/workspaces" },
    {
      label:
        controller.state.workspace?.name ??
        controller.state.workspace?.alias ??
        "Workspace",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}`,
    },
    { label: "Edit" },
  ]}
>
  <UI.AdminContent mode="form">
    {#if controller.state.loading}<UI.Text tone="muted"
        >Loading workspace…</UI.Text
      >{:else if !controller.state.workspace}<UI.Notice
        variant="destructive"
        role="alert"
        >{controller.state.error || "Workspace was not found."}</UI.Notice
      >
    {:else}<UI.AdminHeader
        title={`Edit ${controller.state.workspace.name ?? controller.state.workspace.alias ?? "workspace"}`}
        id={workspaceID}
        description="Update the display name and availability. The alias is permanent."
      />
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <UI.FormSection title="Workspace information"
          ><UI.Text size="small" tone="muted"
            >Alias: {controller.state.workspace.alias ??
              "Not configured"}</UI.Text
          ><UI.Field
            ><UI.Label for="workspace-name">Display name</UI.Label><UI.Input
              id="workspace-name"
              bind:value={controller.state.form.name}
            /></UI.Field
          ></UI.FormSection
        >
        <UI.FormSection title="Availability"
          ><UI.Label layout="choice"
            ><UI.Checkbox
              bind:checked={controller.state.form.enabled}
            />Enabled</UI.Label
          ></UI.FormSection
        >
        {#if controller.state.error}<UI.Notice
            variant="destructive"
            role="alert">{controller.state.error}</UI.Notice
          >{/if}
        <UI.FormActions
          ><UI.RouterLink
            variant="outline"
            href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}`}
            >Cancel</UI.RouterLink
          ><UI.Button type="submit" disabled={controller.state.saving}
            >{controller.state.saving ? "Saving…" : "Save changes"}</UI.Button
          ></UI.FormActions
        >
      </UI.Form>
    {/if}
  </UI.AdminContent>
</UI.AppFrame>
