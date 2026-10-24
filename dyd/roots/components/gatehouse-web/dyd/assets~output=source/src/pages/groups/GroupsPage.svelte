<script lang="ts">
  import * as UI from "../../ui"
  import { untrack, onDestroy } from "svelte"
  import { useRuntime } from "../../app/runtime.svelte"
  import { createSystemGroupController } from "../system/workspaces/system-group-controller.svelte"
  import type { ActivityClient } from "../../app/activity"
  import type { Workspace } from "../../app/access"
  import { createGroupsController } from "./groups-controller.svelte"
  import { routePath } from "../../route"

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
  const runtime = useRuntime()
  const collectionPath = () => routePath({ kind: "group-collection", workspaceID: workspace.id })
  const creating = $derived(runtime.state.route.kind === "group-collection" && runtime.state.route.dialog === "create")
  function closeCreate() {
    if (!form.state.saving) runtime.navigate(collectionPath(), true)
  }
  const form = createSystemGroupController({
    workspaceID: () => workspace.id,
    groupID: () => "",
    scope: "workspace",
    onAuthenticationLost: () => onAuthenticationLost(),
    onSystemAccessChange: () => {},
  })
  onDestroy(form.stop)
  $effect(() => {
    const open = creating
    untrack(() => {
      if (!open) form.cancelMutation()
      else {
        form.clearFeedback("details")
        form.state.error = ""
      }
    })
  })
  async function create() {
    const group = await form.create()
    if (group)
      runtime.navigate(
        `/app/wsp/${encodeURIComponent(workspace.id)}/grp/${encodeURIComponent(group.id)}`,
      )
  }

  $effect(() => {
    const workspaceID = workspace.id
    const routeSignal = signal
    return untrack(() => controller.start(workspaceID, routeSignal))
  })
</script>

<UI.AdminContent>
  <UI.AdminHeader
    title="Groups"
    description="Your group memberships in this workspace."
  >
    {#snippet actions()}{#if controller.state.canCreate}<UI.RouterLink
          variant="default"
          href={routePath({ kind: "group-collection", workspaceID: workspace.id, dialog: "create" })}>Create group</UI.RouterLink
        >{/if}{/snippet}
  </UI.AdminHeader>
  {#if creating && controller.state.hasLoaded && !controller.state.canCreate}
    <UI.Notice role="alert">Workspace admin access is required to create a group.
      <UI.RouterLink variant="outline" href={collectionPath()} replace>Close</UI.RouterLink>
    </UI.Notice>
  {/if}
  {#if creating && controller.state.canCreate}
    <UI.ResourceDialog
      open
      compact
      includeClose={false}
      title="Create group"
      viewKey="create"
      busy={form.state.saving}
      onClose={closeCreate}
    >
      <UI.Form
        id="create-group"
        onsubmit={(event) => {
          event.preventDefault()
          void create()
        }}
      >
        <UI.Field>
          <UI.Label for="create-group-alias">Alias</UI.Label>
          <UI.Input
            id="create-group-alias"
            required
            maxlength={256}
            bind:value={form.state.form.alias}
            disabled={form.state.saving}
          />
          <UI.Text size="small" tone="muted"
            >Lowercase letters, numbers, underscores, and hyphens; start with
            a letter.</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Label for="create-group-name">Display name (optional)</UI.Label>
          <UI.Input
            id="create-group-name"
            maxlength={256}
            bind:value={form.state.form.name}
            disabled={form.state.saving}
          />
        </UI.Field>
        <UI.Text size="small" tone="muted"
          >You will be added as a group admin.</UI.Text
        >
        <UI.ActionFeedback
          error={form.state.error}
          pending={form.state.saving}
          label="Creating group…"
        />
      </UI.Form>
      {#snippet footer()}
        <UI.Button
          variant="outline"
          disabled={form.state.saving}
          onclick={closeCreate}>Cancel</UI.Button
        >
        <UI.ActionButton
          type="submit"
          form="create-group"
          pending={form.state.saving}>Create group</UI.ActionButton
        >
      {/snippet}
    </UI.ResourceDialog>
  {/if}
  <UI.SearchToolbar
    label="Search groups"
    placeholder="Group name or alias"
    bind:value={controller.page.state.search}
    loading={controller.state.busy || controller.page.state.busy}
    onsearch={() =>
      void controller.page.search(() => controller.load(workspace.id, signal))}
  />
  {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
      >{controller.state.error}</UI.Notice
    >{/if}
  {#if controller.state.reloading}<UI.Text role="status"
      >Refreshing groups…</UI.Text
    >{/if}
  {#if controller.state.error}<UI.Button
      variant="outline"
      onclick={() => void controller.load(workspace.id, signal)}
      >Retry</UI.Button
    >{/if}
  <UI.RecordTable headers={["Name", "Your role"]}>
    {#each controller.state.groups as group (group.id)}<UI.RecordRow>
        <UI.RecordCell primary
          ><UI.RouterLink
            href={`/app/wsp/${encodeURIComponent(workspace.id)}/grp/${encodeURIComponent(group.id)}`}
            >{group.name ?? group.alias ?? "Unnamed group"}</UI.RouterLink
          ></UI.RecordCell
        >
        <UI.RecordCell label="Your role"
          >{group.role === "admin" ? "Admin" : "Member"}</UI.RecordCell
        >
      </UI.RecordRow>{:else}<UI.TableEmpty
        columns={2}
        message={!controller.state.hasLoaded && controller.state.error
          ? "Groups could not be loaded."
          : controller.state.loading
            ? "Searching groups…"
            : "No groups match your search."}
      />{/each}
  </UI.RecordTable>
  <UI.PaginationFooter
    previous={controller.page.state.history.length > 0}
    next={!!controller.page.state.next}
    loading={controller.state.busy || controller.page.state.busy}
    onprevious={() =>
      void controller.page.previous(() =>
        controller.load(workspace.id, signal),
      )}
    onnext={() =>
      void controller.page.next(() => controller.load(workspace.id, signal))}
  />
</UI.AdminContent>
