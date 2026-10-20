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
  <UI.AdminContent mode="hub">
    {#if controller.state.reloading}<UI.Text role="status"
        >Refreshing workspace…</UI.Text
      >{/if}
    {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
        >{controller.state.error}<UI.Button
          variant="outline"
          onclick={() => void controller.reload()}>Retry</UI.Button
        ></UI.Notice
      >{/if}
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
      >{:else}<UI.AdminHeader
        kind="Workspace"
        title={controller.state.workspace.name ??
          controller.state.workspace.alias ??
          "Workspace"}
        id={controller.state.workspace.id}
        enabled={controller.state.workspace.enabled}
      >
        {#snippet actions()}
          <UI.RouterLink
            variant="outline"
            href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/edit`}
            >Edit workspace</UI.RouterLink
          >
        {/snippet}
      </UI.AdminHeader>
      <UI.DetailSection title="Workspace information">
        <UI.Property label="Alias"
          >{controller.state.workspace.alias ?? "Not configured"}</UI.Property
        >
      </UI.DetailSection>
      <UI.Group>
        <UI.Heading level={3}>Workspace management</UI.Heading>
        <UI.Text as="p" tone="muted">
          Choose a workspace-specific area to manage.
        </UI.Text>
      </UI.Group>
      <UI.NavigationGrid>
        <UI.NavigationCard
          title="Principals"
          description="Manage direct principal access and workspace roles."
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/principals`}
        />
        <UI.NavigationCard
          title="Groups"
          description="Organize workspace members and bots into groups."
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/groups`}
        />
        <UI.NavigationCard
          title="Agent bindings"
          description="Choose the agent models available in this workspace."
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings`}
        />
        <UI.NavigationCard
          title="Storage bindings"
          description="Choose the storage providers available in this workspace."
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings`}
        />
      </UI.NavigationGrid>
    {/if}
  </UI.AdminContent>
</UI.AppFrame>
