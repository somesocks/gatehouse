<script lang="ts">
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { onMount, untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { createSystemWorkspacesController } from "./system-workspaces-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createSystemWorkspacesController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  onMount(() => controller.start())
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspaces")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspaces" },
  ]}
>
  <UI.Stack as="section">
    <UI.Row justify="between" align="end">
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>Workspaces</UI.Heading>
        <UI.Text as="p">View all workspaces and their enabled status.</UI.Text>
      </UI.Group>
      <UI.RouterLink variant="default" href="/app/system/workspaces/new"
        >Create workspace</UI.RouterLink
      >
    </UI.Row>

    {#if controller.state.error !== ""}<UI.Text
        as="p"
        size="small"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error}
      </UI.Text>{/if}

    <UI.Form
      layout="search"
      onsubmit={(event) => {
        event.preventDefault()
        controller.search()
      }}
    >
      <UI.Label layout="field"
        ><UI.Text>Search workspaces</UI.Text><UI.Input
          id="workspace-search"
          type="search"
          autocomplete="off"
          placeholder="Search by name, alias, or ID"
          bind:value={controller.state.search}
        /></UI.Label
      >
      <UI.Button
        variant="ghost"
        size="icon"
        type="submit"
        aria-label="Search workspaces"
        title="Search workspaces"
        ><UI.Search size={20} strokeWidth={2} aria-hidden="true" /></UI.Button
      >
    </UI.Form>

    {#if controller.state.loading}<UI.Text as="p" tone="muted">
        Loading workspaces...
      </UI.Text>{:else}<UI.List>
        {#each controller.state.workspaces as workspace (workspace.id)}<UI.RouterLink
            variant="item"
            data-disabled={!workspace.enabled || undefined}
            href={`/app/system/workspaces/${encodeURIComponent(workspace.id)}`}
          >
            <UI.Stack gap="small">
              <UI.Text as="strong" emphasis="strong"
                >{workspace.name ?? workspace.alias ?? "Workspace"}</UI.Text
              >
              <UI.Text as="small" size="small"
                >{workspace.alias ?? "No alias"} / {workspace.id}</UI.Text
              >
            </UI.Stack>
            <UI.Text tone={workspace.enabled ? "default" : "muted"}>
              {workspace.enabled ? "Enabled" : "Disabled"}
            </UI.Text>
          </UI.RouterLink>{:else}<UI.Text as="p" tone="muted">
            No workspaces match your search.
          </UI.Text>{/each}
      </UI.List>{/if}
    {#if controller.state.cursor !== null}<UI.Button
        variant="outline"
        size="sm"
        type="button"
        disabled={controller.state.loading}
        onclick={() => controller.loadMore()}
        >{controller.state.loading ? "Loading..." : "Show more"}</UI.Button
      >{/if}
  </UI.Stack>
</UI.AppFrame>
