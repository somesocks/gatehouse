<script lang="ts">
  import { onMount, untrack } from "svelte"
  import * as UI from "../../ui"
  import { createConfirmation } from "../../utils/confirmation.svelte"
  import { useRuntime } from "../../app/runtime.svelte"
  import {
    fetchAdminGroupMembers,
    type SystemGroupMember,
    type GroupScope,
    fetchAdminGroupAgents,
    type SystemGroupAgent,
  } from "../../app/group-administration"
  import { createDirectoryController } from "../../utils/directory-controller.svelte"
  import { createSystemGroupController } from "../system/workspaces/system-group-controller.svelte"
  import SystemGroupDetailsDialog from "../system/workspaces/SystemGroupDetailsDialog.svelte"
  import SystemGroupMembershipDialog from "../system/workspaces/SystemGroupMembershipDialog.svelte"
  import GroupSecretsSection from "./GroupSecretsSection.svelte"
  import GroupMemberRoleDialog from "./GroupMemberRoleDialog.svelte"
  import { routePath } from "../../route"

  let {
    workspaceID,
    groupID,
    scope = "workspace",
  }: { workspaceID: string; groupID: string; scope?: GroupScope } = $props()
  const runtime = useRuntime()
  const confirmation = createConfirmation()
  let editingMember = $state<SystemGroupMember | null>(null)
  let systemDialog = $state<"details" | "members" | "bots" | null>(null)
  const dialog = $derived(scope === "system" ? systemDialog
    : runtime.state.route.kind === "group" && runtime.state.route.groupID === groupID
      ? runtime.state.route.dialog ?? null : null)
  const groupPath = () => routePath({ kind: "group", workspaceID, groupID })
  const dialogPath = (dialog: "details" | "members" | "bots") => routePath({ kind: "group", workspaceID, groupID, dialog })
  function closeDialog() {
    if (scope === "system") systemDialog = null
    else runtime.navigate(groupPath(), true)
  }
  const members = createDirectoryController<SystemGroupMember>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: (next) => {
      if (scope === "system") runtime.access.setSystemAccess(next)
    },
    async fetchPage(name, cursor, _status, signal) {
      const response = await fetchAdminGroupMembers(
        workspaceID,
        groupID,
        name,
        cursor,
        "enabled",
        signal,
        scope,
      )
      const body = response.ok
        ? ((await response.json()) as {
            members: SystemGroupMember[]
            next_cursor?: string
          })
        : undefined
      return { response, items: body?.members, next: body?.next_cursor }
    },
  })
  const agents = createDirectoryController<SystemGroupAgent>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: (next) => {
      if (scope === "system") runtime.access.setSystemAccess(next)
    },
    async fetchPage(name, cursor, _status, signal) {
      const response = await fetchAdminGroupAgents(
        workspaceID,
        groupID,
        name,
        cursor,
        "enabled",
        signal,
        scope,
      )
      const body = response.ok
        ? ((await response.json()) as {
            agents: SystemGroupAgent[]
            next_cursor?: string
          })
        : undefined
      return { response, items: body?.agents, next: body?.next_cursor }
    },
  })
  const controller = untrack(() =>
    createSystemGroupController({
      workspaceID: () => workspaceID,
      groupID: () => groupID,
      scope,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
      onMembersChanged: () => members.reload(),
      onAgentsChanged: () => agents.reload(),
    }),
  )
  const view = controller.state
  const groupLoaded = $derived(view.group !== null)
  let canManage = $derived(scope === "system" || view.group?.role === "admin")
  $effect.pre(() => {
    runtime.state.route
    untrack(() => (editingMember = null))
  })
  $effect(() => {
    if (!canManage) editingMember = null
  })
  onMount(() => {
    void controller.load()
    void members.search()
    void agents.search()
    const unsubscribe =
      scope === "workspace"
        ? runtime.activity.subscribe(
            [
              {
                name: "group",
                topic: workspaceID,
                events: [
                  "group.*",
                  "group_member.*",
                  "group_agent.*",
                  "workspace_grant.*",
                ],
              },
            ],
            async ({ signal }) => {
              if (signal.aborted) return
              const loaded = await controller.load(false, signal)
              if (signal.aborted) return
              if (!loaded) throw new Error("Group refresh failed")
              if (!controller.state.group) {
                members.reset()
                agents.reset()
                if (dialog) closeDialog()
                return
              }
              const refreshed = await Promise.all([
                members.reload(),
                agents.reload(),
              ])
              if (!signal.aborted && refreshed.some((success) => !success))
                throw new Error("Group membership refresh failed")
            },
          )
        : undefined
    return () => {
      unsubscribe?.()
      controller.stop()
      members.stop()
      agents.stop()
    }
  })
  function editDetails() {
    systemDialog = "details"
  }
  function addMembership(kind: "members" | "bots") {
    systemDialog = kind
  }
  $effect(() => {
    const activeDialog = dialog
    const ready = groupLoaded
    return untrack(() => {
      if (!ready || !activeDialog || activeDialog.startsWith("secret-")) return
      if (activeDialog === "details") controller.beginDetails()
      else if (activeDialog === "members" || activeDialog === "bots") {
        view.memberRole = "member"
        controller.clearFeedback(activeDialog)
      }
      return controller.cancelMutation
    })
  })
  function deleteGroup() {
    if (!view.group) return
    const group = { ...view.group }
    confirmation.request({
      title: "Delete group?",
      description: `Delete “${group.name ?? group.alias ?? "Group"}”? Memberships and access inherited through this group will stop applying.`,
      confirmLabel: "Delete group",
      onConfirm: async () => {
        if (await controller.deleteGroup(group))
          runtime.navigate(scope === "system"
            ? `/app/system/workspaces/${encodeURIComponent(workspaceID)}/groups`
            : `/app/wsp/${encodeURIComponent(workspaceID)}/grp`)
        else return view.feedback.details.error || "The group could not be deleted."
      },
    })
  }
  function removeMember(member: SystemGroupMember) {
    const snapshot = { ...member }
    confirmation.request({
      title: "Remove member?",
      description: `Remove ${member.name ?? member.alias ?? "this person"} from the group? Access inherited through this membership will stop applying. Other access paths remain.`,
      confirmLabel: "Remove member",
      onConfirm: async () => {
        if (!(await controller.toggleMember(snapshot)))
          return view.feedback.members.error || "The member could not be removed."
      },
    })
  }
  function removeBot(agent: SystemGroupAgent) {
    const snapshot = { ...agent }
    confirmation.request({
      title: "Remove bot?",
      description: `Remove ${agent.label ?? agent.alias} from the group? The bot will no longer participate through this group.`,
      confirmLabel: "Remove bot",
      onConfirm: async () => {
        if (!(await controller.toggleAgent(snapshot)))
          return view.feedback.bots.error || "The bot could not be removed."
      },
    })
  }
  function editMember(member: SystemGroupMember) {
    controller.clearFeedback("members")
    editingMember = { ...member }
  }
</script>

{#if !view.group}
  <UI.AdminContent mode="detail">
    <UI.Text role="status"
      >{view.loading ? "Loading group…" : "Group could not be loaded."}</UI.Text
    >
    {#if view.loadError}<UI.Notice variant="destructive" role="alert"
        >{view.loadError}<UI.Button
          variant="outline"
          onclick={() => void controller.load()}>Retry</UI.Button
        ></UI.Notice
      >{/if}
  </UI.AdminContent>
{:else}
  {@const group = view.group}
  <UI.ManagementLayout>
    {#snippet header()}
      <UI.AdminHeader kind="Group" title={group.name ?? group.alias ?? "Group"}>
        {#snippet actions()}{#if canManage}
            {#if scope === "workspace"}<UI.RouterLink variant="default" href={dialogPath("details")} disabled={view.saving}>Edit details</UI.RouterLink>
            {:else}<UI.Button
              disabled={view.saving}
              onclick={editDetails}>Edit details</UI.Button
            >{/if}
            {#if group.enabled}<UI.ActionButton
                variant="outline"
                disabled={view.saving}
                pending={view.pendingAction === "details:delete"}
                onclick={() => void deleteGroup()}>Delete group</UI.ActionButton
              >{/if}
          {/if}
        {/snippet}
      </UI.AdminHeader>
      {#if dialog !== "details" && !confirmation.state.request && (view.feedback.details.error || view.loadError)}
        <UI.ActionFeedback
          error={view.loadError || view.feedback.details.error}
        />
      {/if}
      {#if dialog && !canManage}
        <UI.Notice role="alert">Group admin access is required to use this view.
          <UI.RouterLink variant="outline" href={groupPath()} replace>Close</UI.RouterLink>
        </UI.Notice>
      {/if}
    {/snippet}

    <UI.ManagementSection title="Members" loading={members.state.busy}>
      {#snippet actions()}{#if canManage}
          {#if scope === "workspace"}<UI.RouterLink variant="default" href={dialogPath("members")} disabled={view.saving}>Add member</UI.RouterLink>
          {:else}<UI.Button
            disabled={view.saving}
            onclick={() => addMembership("members")}>Add member</UI.Button
          >{/if}{/if}{/snippet}
      {#snippet toolbar()}
        <UI.SearchToolbar
          framed={false}
          label="Search members"
          placeholder="Search members…"
          bind:value={members.state.search}
          loading={members.state.busy}
          onsearch={() => void members.search()}
        />
      {/snippet}
      <UI.RecordTable
        embedded
        headers={canManage ? ["Name", "Role", "Actions"] : ["Name", "Role"]}
      >
        {#each members.state.items as member (member.principal)}
          {@const name = member.name ?? member.alias ?? "Unnamed person"}
          <UI.RecordRow>
            <UI.RecordCell primary>
              {#if scope === "system"}
                <UI.RouterLink variant="record" href={`/app/system/principals/${encodeURIComponent(member.principal)}`}>{name}</UI.RouterLink>
              {:else}
                <UI.Text>{name}</UI.Text>
              {/if}
              {#if member.alias}<UI.Text size="small" tone="muted">@{member.alias}</UI.Text>{/if}
            </UI.RecordCell>
            <UI.RecordCell label="Role">{member.role === "admin" ? "Admin" : "Member"}</UI.RecordCell>
            {#if canManage}
              <UI.RecordCell actions>
                <UI.Row justify="end" wrap={false}>
                  <UI.Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={`Edit ${name}`}
                    title="Edit member role"
                    disabled={view.saving}
                    onclick={() => editMember(member)}
                  ><UI.Pencil aria-hidden="true" /></UI.Button>
                  <UI.Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={`Remove ${name}`}
                    title="Remove member"
                    disabled={view.saving}
                    onclick={() => removeMember(member)}
                  ><UI.Trash2 aria-hidden="true" /></UI.Button>
                </UI.Row>
              </UI.RecordCell>
            {/if}
          </UI.RecordRow>
        {:else}
          <UI.TableEmpty
            columns={canManage ? 3 : 2}
            message={members.state.loading
              ? "Loading members…"
              : members.state.error
                ? "Members could not be loaded."
                : "No memberships match your search."}
          />
        {/each}
      </UI.RecordTable>
      {#snippet footer()}
        <UI.ActionFeedback
          error={members.state.error || (dialog !== "members" && !editingMember && !confirmation.state.request ? view.feedback.members.error : "")}
          notice={view.feedback.members.notice}
          pending={members.state.busy ||
            view.pendingAction.startsWith("member")}
          label={members.state.busy
            ? "Refreshing members…"
            : "Saving membership…"}
        />
        {#if members.state.error}<UI.Row
            ><UI.Button
              variant="outline"
              size="sm"
              onclick={() => void members.retry()}>Reload members</UI.Button
            ></UI.Row
          >{/if}
        <UI.PaginationFooter
          previous={!!members.state.history.length}
          next={!!members.state.next}
          loading={members.state.busy}
          onprevious={() => void members.previous()}
          onnext={() => void members.next()}
        />
      {/snippet}
    </UI.ManagementSection>

    <UI.ManagementSection title="Bots" loading={agents.state.busy}>
      {#snippet actions()}{#if canManage}
          {#if scope === "workspace"}<UI.RouterLink variant="default" href={dialogPath("bots")} disabled={view.saving}>Add bot</UI.RouterLink>
          {:else}<UI.Button
            disabled={view.saving}
            onclick={() => addMembership("bots")}>Add bot</UI.Button
          >{/if}{/if}{/snippet}
      {#snippet toolbar()}
        <UI.SearchToolbar
          framed={false}
          label="Search bots"
          placeholder="Search bots…"
          bind:value={agents.state.search}
          loading={agents.state.busy}
          onsearch={() => void agents.search()}
        />
      {/snippet}
      <UI.RecordList
        label="Group bots"
        framed={false}
        scrollable={false}
        items={agents.state.items.map((agent) => ({
          ...agent,
          id: agent.agent,
          label: agent.label ?? agent.alias,
        }))}
        href={scope === "system"
          ? (agent) =>
              `/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings/${encodeURIComponent(agent.agent)}`
          : undefined}
        emptyMessage={agents.state.loading
          ? "Loading bots…"
          : agents.state.error
            ? "Bots could not be loaded."
            : "No bot memberships match your search."}
      >
        {#snippet details(agent)}@{agent.alias}{/snippet}
        {#snippet actions(agent)}
          {#if canManage}
            <UI.Button
              size="icon-sm"
              variant="ghost"
              aria-label={`Remove ${agent.label ?? agent.alias}`}
              title="Remove bot"
              disabled={view.saving}
              onclick={() => removeBot(agent)}
              ><UI.Trash2 aria-hidden="true" /></UI.Button>
          {/if}
        {/snippet}
      </UI.RecordList>
      {#snippet footer()}
        <UI.ActionFeedback
          error={agents.state.error || (dialog !== "bots" && !confirmation.state.request ? view.feedback.bots.error : "")}
          notice={view.feedback.bots.notice}
          pending={agents.state.busy || view.pendingAction.startsWith("bot")}
          label={agents.state.busy
            ? "Refreshing bots…"
            : "Saving bot membership…"}
        />
        {#if agents.state.error}<UI.Row
            ><UI.Button
              variant="outline"
              size="sm"
              onclick={() => void agents.retry()}>Reload bots</UI.Button
            ></UI.Row
          >{/if}
        <UI.PaginationFooter
          previous={!!agents.state.history.length}
          next={!!agents.state.next}
          loading={agents.state.busy}
          onprevious={() => void agents.previous()}
          onnext={() => void agents.next()}
        />
      {/snippet}
    </UI.ManagementSection>
    {#if scope === "workspace"}
      {#key `${workspaceID}/${groupID}`}
        <GroupSecretsSection {workspaceID} {groupID} />
      {/key}
    {/if}
  </UI.ManagementLayout>
  {#if canManage && dialog === "details"}<SystemGroupDetailsDialog
      {controller}
      onClose={closeDialog}
    />
  {:else if canManage && (dialog === "members" || dialog === "bots")}<SystemGroupMembershipDialog
      {controller}
      {workspaceID}
      {groupID}
      {scope}
      kind={dialog}
      onClose={closeDialog}
    />
  {/if}
  {#if canManage && editingMember}
    {@const member = editingMember}
    <GroupMemberRoleDialog
      {controller}
      {member}
      onReload={async () => {
        if (!(await members.reload())) return undefined
        return members.state.items.find((candidate) => candidate.principal === member.principal)
      }}
      onClose={() => (editingMember = null)}
    />
  {/if}
{/if}

<UI.ConfirmationDialog {confirmation} contextKey={runtime.state.route} />
