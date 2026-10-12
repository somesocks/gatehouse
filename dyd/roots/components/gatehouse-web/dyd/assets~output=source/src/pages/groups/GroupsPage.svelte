<script lang="ts">
  import { PageBody, PageHeading } from "../../ui"
  import * as UI from "../../ui"
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../app/activity"
  import type { Workspace } from "../../app/access"
  import { createGroupsController } from "./groups-controller.svelte"

  let {
    workspace,
    signal,
    activity,
    onAuthenticationLost,
  }: {
    workspace: Workspace
    signal: AbortSignal
    activity: ActivityClient
    onAuthenticationLost: () => void
  } = $props()
  const controller = untrack(() =>
    createGroupsController({ activity, onAuthenticationLost }),
  )

  $effect(() => {
    const workspaceID = workspace.id
    const routeSignal = signal
    return untrack(() => controller.start(workspaceID, routeSignal))
  })
</script>

<UI.AdminContent>
  <UI.AdminHeader
    title="Groups"
    description="Your enabled group memberships in this workspace."
  />
  <UI.SearchToolbar
    label="Search groups"
    placeholder="Group name or alias"
    bind:value={controller.page.state.search}
    loading={controller.state.loading || controller.page.state.busy}
    onsearch={() =>
      void controller.page.search(() => controller.load(workspace.id, signal))}
  />
  {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
      >{controller.state.error}</UI.Notice
    >{/if}
  <UI.RecordTable headers={["Name"]}>
    {#each controller.state.groups as group (group.id)}<UI.RecordRow>
        <UI.RecordCell primary>{group.name ?? "Unnamed group"}</UI.RecordCell>
      </UI.RecordRow>{:else}<UI.TableEmpty
        columns={1}
        message={controller.state.loading
          ? "Searching groups…"
          : "No groups match your search."}
      />{/each}
  </UI.RecordTable>
  <UI.PaginationFooter
    noun="groups"
    count={controller.state.groups.length}
    previous={controller.page.state.history.length > 0}
    next={!!controller.page.state.next}
    loading={controller.state.loading || controller.page.state.busy}
    onprevious={() =>
      void controller.page.previous(() =>
        controller.load(workspace.id, signal),
      )}
    onnext={() =>
      void controller.page.next(() => controller.load(workspace.id, signal))}
  />
</UI.AdminContent>
