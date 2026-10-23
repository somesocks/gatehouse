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

  let {
    workspaceID,
    groupID,
    scope = "workspace",
  }: { workspaceID: string; groupID: string; scope?: GroupScope } = $props()
  const runtime = useRuntime()
  const confirmation = createConfirmation()
  let dialog = $state<"details" | "members" | "bots" | null>(null)
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
  let canManage = $derived(scope === "system" || view.group?.role === "manager")
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
                dialog = null
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
    controller.beginDetails()
    dialog = "details"
  }
  function addMembership(kind: "members" | "bots") {
    view.memberRole = "member"
    controller.clearFeedback(kind)
    dialog = kind
  }
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
  function changeMemberRole(member: SystemGroupMember, role: "member" | "manager") {
    if (member.role !== "manager" || role !== "member") {
      void controller.changeMemberRole(member, role)
      return
    }
    const snapshot = { ...member }
    confirmation.request({
      title: "Remove group management access?",
      description: `${member.name ?? member.alias ?? "This person"} will remain a member but will no longer be able to manage this group.`,
      confirmLabel: "Change to member",
      onConfirm: async () => {
        if (!(await controller.changeMemberRole(snapshot, role)))
          return view.feedback.members.error || "The role could not be changed."
      },
    })
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
        {#snippet actions()}{#if canManage}<UI.Button
              variant="outline"
              disabled={view.saving}
              onclick={editDetails}>Edit details</UI.Button
            >
            {#if group.enabled}<UI.ActionButton
                variant="outline"
                disabled={view.saving}
                pending={view.pendingAction === "details:delete"}
                onclick={() => void deleteGroup()}>Delete group</UI.ActionButton
              >{/if}
          {/if}
        {/snippet}
      </UI.AdminHeader>
      {#if view.feedback.details.error || view.loadError}
        <UI.ActionFeedback
          error={view.feedback.details.error || view.loadError}
        />
      {/if}
    {/snippet}

    <UI.ManagementSection title="Members" loading={members.state.busy}>
      {#snippet actions()}{#if canManage}<UI.Button
            size="sm"
            variant="outline"
            disabled={view.saving}
            onclick={() => addMembership("members")}>Add member</UI.Button
          >{/if}{/snippet}
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
      <UI.RecordList
        label="Group members"
        framed={false}
        scrollable={false}
        items={members.state.items.map((member) => ({
          ...member,
          id: member.principal,
          label: member.name ?? member.alias ?? "Unnamed person",
        }))}
        href={scope === "system"
          ? (member) =>
              `/app/system/principals/${encodeURIComponent(member.principal)}`
          : undefined}
        emptyMessage={members.state.loading
          ? "Loading members…"
          : members.state.error
            ? "Members could not be loaded."
            : "No memberships match your search."}
      >
        {#snippet details(
          member,
        )}{#if member.alias}@{member.alias}{/if}{/snippet}
        {#snippet actions(member)}
          {#if canManage}
            <UI.Select
              aria-label={`Role for ${member.label}`}
              value={member.role}
              disabled={view.saving}
              onchange={(event) => {
                const role = event.currentTarget.value as "member" | "manager"
                event.currentTarget.value = member.role
                changeMemberRole(member, role)
              }}
            >
              <UI.Option value="member">Member</UI.Option>
              <UI.Option value="manager">Manager</UI.Option>
            </UI.Select>
            <UI.ActionButton
              size="sm"
              variant="ghost"
              aria-label={`Remove ${member.label}`}
              pending={view.pendingAction === `member:${member.principal}`}
              disabled={view.saving}
              onclick={() => removeMember(member)}
              >Remove</UI.ActionButton
            >
          {:else}
            <UI.Text size="small" tone="muted"
              >{member.role === "manager" ? "Manager" : "Member"}</UI.Text
            >
          {/if}
        {/snippet}
      </UI.RecordList>
      {#snippet footer()}
        <UI.ActionFeedback
          error={view.feedback.members.error || members.state.error}
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
      {#snippet actions()}{#if canManage}<UI.Button
            size="sm"
            variant="outline"
            disabled={view.saving}
            onclick={() => addMembership("bots")}>Add bot</UI.Button
          >{/if}{/snippet}
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
            <UI.ActionButton
              size="sm"
              variant="ghost"
              aria-label={`Remove ${agent.label}`}
              pending={view.pendingAction === `bot:${agent.agent}`}
              disabled={view.saving}
              onclick={() => removeBot(agent)}
              >Remove</UI.ActionButton
            >
          {/if}
        {/snippet}
      </UI.RecordList>
      {#snippet footer()}
        <UI.ActionFeedback
          error={view.feedback.bots.error || agents.state.error}
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
      onClose={() => (dialog = null)}
    />
  {:else if canManage && (dialog === "members" || dialog === "bots")}<SystemGroupMembershipDialog
      {controller}
      {workspaceID}
      {groupID}
      {scope}
      kind={dialog}
      onClose={() => (dialog = null)}
    />
  {/if}
{/if}

<UI.ConfirmationDialog {confirmation} contextKey={runtime.state.route} />
