<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    fetchSystemWorkspaces,
    type SystemWorkspace,
  } from "../../../app/system"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import { onMount } from "svelte"
  const runtime = useRuntime()
  const controller = createDirectoryController<SystemWorkspace>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await fetchSystemWorkspaces(name, cursor, signal)
      const result = response.ok
        ? ((await response.json()) as {
            workspaces: SystemWorkspace[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.workspaces, next: result?.next_cursor }
    },
  })
  onMount(() => {
    void controller.search()
    return controller.stop
  })
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspaces")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspaces" },
  ]}
>
  <UI.AdminContent
    ><UI.AdminHeader
      title="Workspaces"
      description="Manage workspace details, principal access, and bindings."
      >{#snippet actions()}<UI.RouterLink
          variant="default"
          href="/app/system/workspaces/new">Create workspace</UI.RouterLink
        >{/snippet}</UI.AdminHeader
    >
    <UI.SearchToolbar
      bind:value={controller.state.search}
      loading={controller.state.loading}
      onsearch={() => void controller.search()}
    />
    {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
        >{controller.state.error}</UI.Notice
      >{/if}
    <UI.RecordTable headers={["Workspace", "Alias", "Status"]}
      >{#if controller.state.loading}<UI.TableEmpty
          columns={3}
          message="Loading workspaces…"
        />{:else}{#each controller.state.items as workspace (workspace.id)}<UI.RecordRow
          >
            <UI.RecordCell primary
              ><UI.RouterLink
                href={`/app/system/workspaces/${encodeURIComponent(workspace.id)}`}
                >{workspace.name ??
                  workspace.alias ??
                  "Workspace"}</UI.RouterLink
              ><UI.Identifier
                value={workspace.id}
                copyable={false}
              /></UI.RecordCell
            >
            <UI.RecordCell label="Alias"
              >{workspace.alias ?? "Not configured"}</UI.RecordCell
            ><UI.RecordCell label="Status"
              ><UI.StatusBadge enabled={workspace.enabled} /></UI.RecordCell
            >
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={3}
            message="No workspaces match your search."
          />{/each}{/if}</UI.RecordTable
    >
    <UI.PaginationFooter
      count={controller.state.items.length}
      previous={!!controller.state.history.length}
      next={!!controller.state.next}
      loading={controller.state.loading}
      onprevious={() => void controller.previous()}
      onnext={() => void controller.next()}
    />
  </UI.AdminContent>
</UI.AppFrame>
