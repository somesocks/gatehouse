<script lang="ts">
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { onDestroy, untrack } from "svelte"
  import { createWorkspaceStorageBindingsController } from "./workspace-storage-bindings-controller.svelte"
  import StorageProviderPicker from "../storage-providers/StorageProviderPicker.svelte"
  import SystemWorkspaceFrame from "../workspaces/SystemWorkspaceFrame.svelte"
  let { workspaceID }: { workspaceID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createWorkspaceStorageBindingsController({
      activity: runtime.activity,
      workspaceID,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  onDestroy(controller.stop)
  controller.state.form.priority = 1
  async function create() {
    const route = runtime.state.route
    await controller.create()
    if (!controller.state.error && runtime.state.route === route)
      runtime.navigate(
        `/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings`,
        true,
      )
  }
</script>

<SystemWorkspaceFrame
  {workspaceID}
  breadcrumbs={[
    {
      label: "Storage bindings",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings`,
    },
    { label: "Add" },
  ]}
>
  <UI.AdminContent mode="form"
    ><UI.AdminHeader
      title="Add storage binding"
      description="Assign a storage provider and set its priority in this workspace."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
      ><UI.FormSection title="Provider assignment"
        ><StorageProviderPicker
          bind:value={controller.state.form.provider}
        /></UI.FormSection
      ><UI.FormSection title="Priority"
        ><UI.Field
          ><UI.Label for="storage-priority">Priority</UI.Label><UI.Input
            id="storage-priority"
            type="number"
            min="1"
            required
            bind:value={controller.state.form.priority}
          /></UI.Field
        ></UI.FormSection
      ><UI.FormSection title="Availability"
        ><UI.Label layout="choice"
          ><UI.Checkbox
            bind:checked={controller.state.form.enabled}
          />Enabled</UI.Label
        ></UI.FormSection
      >{#if controller.state.error}<UI.Notice variant="destructive" role="alert"
          >{controller.state.error}</UI.Notice
        >{/if}<UI.FormActions
        ><UI.RouterLink
          variant="outline"
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings`}
          >Cancel</UI.RouterLink
        ><UI.Button
          type="submit"
          disabled={controller.state.saving || !controller.state.form.provider}
          >{controller.state.saving
            ? "Adding…"
            : "Add storage binding"}</UI.Button
        ></UI.FormActions
      ></UI.Form
    >
  </UI.AdminContent>
</SystemWorkspaceFrame>
