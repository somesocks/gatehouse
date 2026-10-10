<script lang="ts">
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
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
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspaces")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspaces", href: "/app/system/workspaces" },
    { label: controller.state.workspace?.name ?? "Workspace" },
  ]}
>
  <UI.Stack as="section">
    {#if controller.state.loading}<UI.Text as="p" tone="muted">
        Loading workspace...
      </UI.Text>{:else if controller.state.workspace === null}<UI.Text
        as="p"
        size="small"
        role="alert"
      >
        {controller.state.error || "Workspace was not found."}
      </UI.Text><UI.RouterLink variant="outline" href="/app/system/workspaces"
        >Back to workspaces</UI.RouterLink
      >{:else if !controller.state.editing}<UI.Row justify="between">
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Workspace</UI.Text>
          <UI.Heading level={2}
            >{controller.state.workspace.name ??
              controller.state.workspace.alias ??
              "Workspace"}</UI.Heading
          >
          <UI.Text as="p">{controller.state.workspace.id}</UI.Text>
        </UI.Group>
        <UI.Button
          variant="outline"
          type="button"
          onclick={controller.beginEdit}>Edit workspace</UI.Button
        >
      </UI.Row>
      <UI.Stack as="dl">
        <UI.Group as="dt">Alias</UI.Group>
        <UI.Group as="dd"
          >{controller.state.workspace.alias ?? "No alias"}</UI.Group
        >
        <UI.Group as="dt">Display name</UI.Group>
        <UI.Group as="dd"
          >{controller.state.workspace.name ?? "No display name"}</UI.Group
        >
        <UI.Group as="dt">Status</UI.Group>
        <UI.Group as="dd"
          >{controller.state.workspace.enabled
            ? "Enabled"
            : "Disabled"}</UI.Group
        >
      </UI.Stack>
    {:else}<UI.Group>
        <UI.Text as="p" size="eyebrow">Workspace</UI.Text>
        <UI.Heading level={2}>Edit workspace</UI.Heading>
        <UI.Text as="p">Alias and ID are stable identifiers.</UI.Text>
      </UI.Group>
      <UI.Form
        onsubmit={(event) => {
          event.preventDefault()
          void controller.save(workspaceID)
        }}
      >
        <UI.Field>
          <UI.Label for="workspace-alias">Alias</UI.Label>
          <UI.Input
            id="workspace-alias"
            value={controller.state.workspace.alias ?? "No alias"}
            disabled
          />
        </UI.Field>
        <UI.Field>
          <UI.Label for="workspace-name">Display name</UI.Label>
          <UI.Input
            id="workspace-name"
            autocomplete="off"
            bind:value={controller.state.form.name}
          />
        </UI.Field>
        <UI.Row align="center">
          <UI.Checkbox
            id="workspace-enabled"
            bind:checked={controller.state.form.enabled}
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
            {controller.state.saving ? "Saving..." : "Save changes"}
          </UI.Button>
          <UI.Button
            variant="outline"
            type="button"
            disabled={controller.state.saving}
            onclick={controller.cancelEdit}>Cancel</UI.Button
          >
        </UI.Row>
      </UI.Form>
    {/if}
  </UI.Stack>
</UI.AppFrame>
