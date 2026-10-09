<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createWorkspaceStorageBindingsController } from "./workspace-storage-bindings-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createWorkspaceStorageBindingsController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )

  async function create(): Promise<void> {
    await controller.create()
    if (!controller.state.error)
      runtime.navigate("/app/system/workspace-storage-bindings", true)
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspace-storage-bindings")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "New workspace storage binding" },
  ]}
>
  <UI.Stack as="section">
    <UI.Stack>
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>New workspace storage binding</UI.Heading>
      </UI.Group>
    </UI.Stack>
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <UI.Field>
        <UI.Label for="storage-binding-workspace">Workspace ID</UI.Label>
        <UI.Group>
          <UI.Input
            id="storage-binding-workspace"
            required
            bind:value={controller.state.form.workspace}
          />
        </UI.Group>
      </UI.Field>
      <UI.Field>
        <UI.Label for="storage-binding-provider">Storage provider ID</UI.Label>
        <UI.Group>
          <UI.Input
            id="storage-binding-provider"
            required
            bind:value={controller.state.form.provider}
          />
        </UI.Group>
      </UI.Field>
      <UI.Field>
        <UI.Label for="storage-binding-priority">Priority</UI.Label>
        <UI.Group>
          <UI.Input
            id="storage-binding-priority"
            type="number"
            bind:value={controller.state.form.priority}
          />
        </UI.Group>
      </UI.Field>
      <UI.Row>
        <UI.Group>
          <UI.Button variant="default" type="submit">Add binding</UI.Button>
        </UI.Group>
        <UI.Group>
          <RouterLink
            variant="outline"
            href="/app/system/workspace-storage-bindings">Cancel</RouterLink
          >
        </UI.Group>
      </UI.Row>
    </UI.Form>
    {#if controller.state.error}<UI.Text as="p" size="small" role="alert">
        {controller.state.error}
      </UI.Text>{/if}
  </UI.Stack>
</UI.AppFrame>
