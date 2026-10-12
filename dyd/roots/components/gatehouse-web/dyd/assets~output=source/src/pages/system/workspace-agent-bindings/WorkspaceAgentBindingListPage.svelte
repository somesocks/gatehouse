<script lang="ts">
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    fetchSystemWorkspaceAgents,
    type SystemWorkspaceAgent,
  } from "../../../app/system"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import SystemWorkspaceFrame from "../workspaces/SystemWorkspaceFrame.svelte"
  import { onMount } from "svelte"
  let { workspaceID }: { workspaceID: string } = $props()
  const runtime = useRuntime()
  const controller = createDirectoryController<SystemWorkspaceAgent>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await fetchSystemWorkspaceAgents(
        workspaceID,
        name,
        cursor,
        signal,
      )
      const result = response.ok
        ? ((await response.json()) as {
            agents: SystemWorkspaceAgent[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.agents, next: result?.next_cursor }
    },
  })
  onMount(() => {
    void controller.search()
    return controller.stop
  })
</script>

<SystemWorkspaceFrame {workspaceID} breadcrumbs={[{ label: "Agent bindings" }]}>
  <UI.AdminContent
    ><UI.AdminHeader
      title="Agent bindings"
      description="Models and reply behavior assigned to this workspace."
      >{#snippet actions()}<UI.RouterLink
          variant="default"
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings/new`}
          >Add agent binding</UI.RouterLink
        >{/snippet}</UI.AdminHeader
    >
    <UI.SearchToolbar
      bind:value={controller.state.search}
      loading={controller.state.busy}
      placeholder="Alias, label, model, or ID"
      onsearch={() => void controller.search()}
    />
    {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
        >{controller.state.error}</UI.Notice
      >{/if}
    <UI.RecordTable headers={["Agent", "Model", "Replies", "Status"]}
      >{#if !controller.state.hasLoaded}<UI.TableEmpty
          columns={4}
          message={controller.state.error
            ? "Bindings could not be loaded."
            : "Loading bindings…"}
        />{:else}{#each controller.state.items as binding (binding.id)}<UI.RecordRow
          >
            <UI.RecordCell primary
              ><UI.RouterLink
                href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings/${encodeURIComponent(binding.id)}`}
                >{binding.label ?? binding.alias}</UI.RouterLink
              ><UI.Text as="small" size="small" tone="muted"
                >{binding.alias}</UI.Text
              ></UI.RecordCell
            >
            <UI.RecordCell label="Model"
              ><UI.RouterLink
                href={`/app/system/agent-models/${encodeURIComponent(binding.model)}`}
                >{binding.model_alias ?? binding.model}</UI.RouterLink
              ></UI.RecordCell
            ><UI.RecordCell label="Replies"
              ><UI.Badge variant={binding.default ? "secondary" : "outline"}
                >{binding.default ? "Default" : "Manual"}</UI.Badge
              ></UI.RecordCell
            ><UI.RecordCell label="Status"
              ><UI.StatusBadge enabled={binding.enabled} /></UI.RecordCell
            >
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={4}
            message="No bindings match your search."
          />{/each}{/if}</UI.RecordTable
    >
    <UI.PaginationFooter
      count={controller.state.items.length}
      previous={!!controller.state.history.length}
      next={!!controller.state.next}
      loading={controller.state.busy}
      onprevious={() => void controller.previous()}
      onnext={() => void controller.next()}
    />
  </UI.AdminContent>
</SystemWorkspaceFrame>
