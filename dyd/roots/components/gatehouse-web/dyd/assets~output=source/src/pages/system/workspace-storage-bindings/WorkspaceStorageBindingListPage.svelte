<script lang="ts">
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    fetchSystemWorkspaceStorageBindings,
    type SystemWorkspaceStorageProvider,
  } from "../../../app/system"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import SystemWorkspaceFrame from "../workspaces/SystemWorkspaceFrame.svelte"
  import { onMount } from "svelte"
  let { workspaceID }: { workspaceID: string } = $props()
  const runtime = useRuntime()
  const controller = createDirectoryController<SystemWorkspaceStorageProvider>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await fetchSystemWorkspaceStorageBindings(
        workspaceID,
        name,
        cursor,
        signal,
      )
      const result = response.ok
        ? ((await response.json()) as {
            bindings: SystemWorkspaceStorageProvider[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.bindings, next: result?.next_cursor }
    },
  })
  onMount(() => {
    void controller.search()
    return controller.stop
  })
</script>

<SystemWorkspaceFrame
  {workspaceID}
  breadcrumbs={[{ label: "Storage bindings" }]}
>
  <UI.AdminContent
    ><UI.AdminHeader
      title="Storage bindings"
      description="Storage providers and priorities assigned to this workspace."
      >{#snippet actions()}<UI.RouterLink
          variant="default"
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings/new`}
          >Add storage binding</UI.RouterLink
        >{/snippet}</UI.AdminHeader
    >
    <UI.SearchToolbar
      bind:value={controller.state.search}
      loading={controller.state.loading}
      placeholder="Provider alias or ID"
      onsearch={() => void controller.search()}
    />
    {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
        >{controller.state.error}</UI.Notice
      >{/if}
    <UI.RecordTable headers={["Provider", "Priority", "Status"]}
      >{#if controller.state.loading}<UI.TableEmpty
          columns={3}
          message="Loading bindings…"
        />{:else}{#each controller.state.items as binding (binding.provider)}<UI.RecordRow
          >
            <UI.RecordCell primary
              ><UI.RouterLink
                href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings/${encodeURIComponent(binding.provider)}`}
                >{binding.provider_alias ?? binding.provider}</UI.RouterLink
              ><UI.Identifier
                value={binding.provider}
                copyable={false}
              /></UI.RecordCell
            ><UI.RecordCell label="Priority">{binding.priority}</UI.RecordCell
            ><UI.RecordCell label="Status"
              ><UI.StatusBadge enabled={binding.enabled} /></UI.RecordCell
            >
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={3}
            message="No bindings match your search."
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
</SystemWorkspaceFrame>
