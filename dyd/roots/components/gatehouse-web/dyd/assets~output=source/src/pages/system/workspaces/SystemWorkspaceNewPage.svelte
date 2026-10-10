<script lang="ts">
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { createSystemWorkspaceFormController } from "./system-workspaces-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createSystemWorkspaceFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )

  async function create(): Promise<void> {
    if (await controller.create())
      runtime.navigate("/app/system/workspaces", true)
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspaces")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspaces", href: "/app/system/workspaces" },
    { label: "Create workspace" },
  ]}
>
  <UI.Stack as="section">
    <UI.Group>
      <UI.Text as="p" size="eyebrow">System</UI.Text>
      <UI.Heading level={2}>Create workspace</UI.Heading>
      <UI.Text as="p">Set up a workspace and its initial status.</UI.Text>
    </UI.Group>
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <UI.Text as="p" tone="muted"
        >Creating a workspace does not grant you workspace membership.</UI.Text
      >
      <UI.Field>
        <UI.Label for="workspace-alias">Alias</UI.Label>
        <UI.Input
          id="workspace-alias"
          autocomplete="off"
          required
          pattern="[a-z][a-z0-9_-]*"
          bind:value={controller.state.alias}
        />
        <UI.Text as="small" size="small"
          >Use lowercase letters, numbers, underscores, or hyphens.</UI.Text
        >
      </UI.Field>
      <UI.Field>
        <UI.Label for="workspace-name">Display name</UI.Label>
        <UI.Input
          id="workspace-name"
          autocomplete="off"
          bind:value={controller.state.name}
        />
      </UI.Field>
      <UI.Row align="center">
        <UI.Checkbox
          id="workspace-enabled"
          bind:checked={controller.state.enabled}
        />
        <UI.Label for="workspace-enabled">Enabled</UI.Label>
      </UI.Row>
      {#if controller.state.error !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </UI.Text>{/if}
      <UI.Row>
        <UI.Button type="submit" disabled={controller.state.saving}>
          {controller.state.saving ? "Creating..." : "Create workspace"}
        </UI.Button>
        <UI.RouterLink variant="outline" href="/app/system/workspaces"
          >Cancel</UI.RouterLink
        >
      </UI.Row>
    </UI.Form>
  </UI.Stack>
</UI.AppFrame>
