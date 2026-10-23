<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    fetchAdminGroups,
    type SystemGroup,
  } from "../../../app/group-administration"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import { createSystemGroupController } from "./system-group-controller.svelte"
  import SystemWorkspaceFrame from "./SystemWorkspaceFrame.svelte"
  let { workspaceID }: { workspaceID: string } = $props()
  const runtime = useRuntime()
  let creating = $state(false)
  const groups = createDirectoryController<SystemGroup>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await fetchAdminGroups(
        workspaceID,
        name,
        cursor,
        status,
        signal,
      )
      const result = response.ok
        ? ((await response.json()) as {
            groups: SystemGroup[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.groups, next: result?.next_cursor }
    },
  })
  const form = createSystemGroupController({
    workspaceID: () => workspaceID,
    groupID: () => "",
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
  })
  onMount(() => {
    void groups.search()
    return () => {
      groups.stop()
      form.stop()
    }
  })
  async function create() {
    const group = await form.create()
    if (group)
      runtime.navigate(
        `/app/system/workspaces/${encodeURIComponent(workspaceID)}/groups/${encodeURIComponent(group.id)}`,
      )
  }
</script>

<SystemWorkspaceFrame {workspaceID} breadcrumbs={[{ label: "Groups" }]}>
  <UI.AdminContent>
    <UI.AdminHeader
      title="Groups"
      description="Organize workspace members and bots into collaboration groups."
    >
      {#snippet actions()}<UI.Button variant={creating ? "outline" : "default"}
          onclick={() => (creating = !creating)}
          disabled={form.state.saving}
          >{creating ? "Cancel creation" : "Create group"}</UI.Button
        >{/snippet}
    </UI.AdminHeader>
    {#if creating}
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          void create()
        }}
      >
        <UI.FormSection title="Create group">
          <UI.Field
            ><UI.Label for="group-alias">Alias</UI.Label><UI.Input
              id="group-alias"
              required
              maxlength={256}
              bind:value={form.state.form.alias}
              disabled={form.state.saving}
            /><UI.Text size="small" tone="muted"
              >Lowercase letters, numbers, underscores, and hyphens; start with
              a letter.</UI.Text
            ></UI.Field
          >
          <UI.Field
            ><UI.Label for="group-name">Display name</UI.Label><UI.Input
              id="group-name"
              maxlength={256}
              bind:value={form.state.form.name}
              disabled={form.state.saving}
            /></UI.Field
          >
          <UI.Label layout="choice"
            ><UI.Checkbox
              bind:checked={form.state.form.enabled}
              disabled={form.state.saving}
            />Enabled</UI.Label
          >
        </UI.FormSection>
        {#if form.state.error}<UI.Notice variant="destructive" role="alert"
            >{form.state.error}</UI.Notice
          >{/if}
        <UI.FormActions
          ><UI.Button type="submit" disabled={form.state.saving}
            >{form.state.saving ? "Creating…" : "Create group"}</UI.Button
          ></UI.FormActions
        >
      </UI.Form>
    {/if}
    <UI.SearchToolbar
      label="Search groups"
      placeholder="Name or alias"
      bind:value={groups.state.search}
      loading={groups.state.busy}
      onsearch={() => void groups.search()}
    />
    <UI.Field
      ><UI.Label for="groups-status">Status</UI.Label><UI.Select
        id="groups-status"
        bind:value={groups.state.status}
        onchange={(event) => {
          groups.state.status = event.currentTarget.value
          void groups.search()
        }}
        ><UI.Option value="">All groups</UI.Option><UI.Option value="enabled"
          >Enabled</UI.Option
        ><UI.Option value="disabled">Disabled</UI.Option></UI.Select
      ></UI.Field
    >
    {#if groups.state.error}<UI.Notice variant="destructive" role="alert"
        >{groups.state.error}<UI.Button
          variant="outline"
          onclick={() => void groups.retry()}>Retry</UI.Button
        ></UI.Notice
      >{/if}
    <UI.RecordTable headers={["Group", "Status", "Revision"]}>
      {#each groups.state.items as group (group.id)}
        <UI.RecordRow
          ><UI.RecordCell primary
            ><UI.RouterLink
              href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/groups/${encodeURIComponent(group.id)}`}
              >{group.name ?? group.alias ?? group.id}</UI.RouterLink
            ><UI.Text size="small" tone="muted"
              >{group.alias ?? "No alias"}</UI.Text
            ><UI.Identifier value={group.id} /></UI.RecordCell
          ><UI.RecordCell label="Status"
            ><UI.StatusBadge enabled={group.enabled} /></UI.RecordCell
          ><UI.RecordCell label="Revision">{group.revision}</UI.RecordCell
          ></UI.RecordRow
        >
      {:else}<UI.TableEmpty
          columns={3}
          message={groups.state.busy
            ? "Loading groups…"
            : "No groups match your search."}
        />{/each}
    </UI.RecordTable>
    <UI.PaginationFooter
      count={groups.state.items.length}
      previous={!!groups.state.history.length}
      next={!!groups.state.next}
      loading={groups.state.busy}
      onprevious={() => void groups.previous()}
      onnext={() => void groups.next()}
    />
  </UI.AdminContent>
</SystemWorkspaceFrame>
