<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    fetchAdminGroupMembers,
    type SystemGroupMember,
    fetchAdminGroupAgents,
    type SystemGroupAgent,
  } from "../../../app/group-administration"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import SystemWorkspaceFrame from "./SystemWorkspaceFrame.svelte"
  import { createSystemGroupController } from "./system-group-controller.svelte"
  import SystemGroupDetailsDialog from "./SystemGroupDetailsDialog.svelte"
  import SystemGroupMembershipDialog from "./SystemGroupMembershipDialog.svelte"

  let { workspaceID, groupID }: { workspaceID: string; groupID: string } =
    $props()
  const runtime = useRuntime()
  let dialog = $state<"details" | "members" | "bots" | null>(null)
  const members = createDirectoryController<SystemGroupMember>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    async fetchPage(name, cursor, status, signal) {
      const response = await fetchAdminGroupMembers(
        workspaceID,
        groupID,
        name,
        cursor,
        status,
        signal,
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
    onSystemAccessChange: runtime.access.setSystemAccess,
    async fetchPage(name, cursor, status, signal) {
      const response = await fetchAdminGroupAgents(
        workspaceID,
        groupID,
        name,
        cursor,
        status,
        signal,
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
  const controller = createSystemGroupController({
    workspaceID: () => workspaceID,
    groupID: () => groupID,
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    onMembersChanged: () => members.reload(),
    onAgentsChanged: () => agents.reload(),
  })
  const view = controller.state
  onMount(() => {
    void controller.load()
    void members.search()
    void agents.search()
    return () => {
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
    controller.clearFeedback(kind)
    dialog = kind
  }
</script>

<SystemWorkspaceFrame
  {workspaceID}
  breadcrumbs={[
    {
      label: "Groups",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}/groups`,
    },
    { label: view.group?.name ?? view.group?.alias ?? "Group" },
  ]}
>
  {#if !view.group}
    <UI.AdminContent mode="detail">
      <UI.Text role="status"
        >{view.loading
          ? "Loading group…"
          : "Group could not be loaded."}</UI.Text
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
        <UI.AdminHeader
          kind="Group"
          title={group.name ?? group.alias ?? group.id}
          enabled={group.enabled}
        >
          {#snippet actions()}<UI.Button
              variant="outline"
              disabled={view.saving}
              onclick={editDetails}>Edit details</UI.Button
            >{/snippet}
          {#snippet metadata()}
            <UI.Row tone="muted">
              {#if group.alias}<UI.Text size="small">@{group.alias}</UI.Text
                >{/if}
              <UI.Identifier value={group.id} />
              <UI.Text size="small">Revision {group.revision}</UI.Text>
            </UI.Row>
          {/snippet}
        </UI.AdminHeader>
      {/snippet}

      <UI.ManagementSection title="Members" loading={members.state.busy}>
        {#snippet actions()}<UI.Button
            size="sm"
            variant="outline"
            disabled={view.saving}
            onclick={() => addMembership("members")}>Add member</UI.Button
          >{/snippet}
        {#snippet toolbar()}
          <UI.SearchToolbar
            framed={false}
            label="Search members"
            placeholder="Search members…"
            bind:value={members.state.search}
            loading={members.state.busy}
            onsearch={() => void members.search()}
          >
            {#snippet filters()}
              <UI.Select
                aria-label="Membership status"
                bind:value={members.state.status}
                onchange={() => void members.search()}
              >
                <UI.Option value="">All memberships</UI.Option><UI.Option
                  value="enabled">Enabled</UI.Option
                ><UI.Option value="disabled">Disabled</UI.Option>
              </UI.Select>
            {/snippet}
          </UI.SearchToolbar>
        {/snippet}
        <UI.RecordList
          label="Group members"
          framed={false}
          scrollable={false}
          items={members.state.items.map((member) => ({
            ...member,
            id: member.principal,
            label: member.name ?? member.alias ?? member.principal,
          }))}
          href={(member) =>
            `/app/system/principals/${encodeURIComponent(member.principal)}`}
          emptyMessage={members.state.loading
            ? "Loading members…"
            : members.state.error
              ? "Members could not be loaded."
              : "No memberships match your search."}
        >
          {#snippet details(member)}{member.alias
              ? `@${member.alias}`
              : member.principal}{/snippet}
          {#snippet badges(member)}{#if !member.enabled}<UI.Badge
                >Membership disabled</UI.Badge
              >{/if}{#if !member.principal_enabled}<UI.Badge
                >Account disabled</UI.Badge
              >{/if}{/snippet}
          {#snippet actions(member)}
            <UI.ActionButton
              size="sm"
              variant="ghost"
              aria-label={`${member.enabled ? "Remove" : "Enable"} membership for ${member.label}`}
              pending={view.pendingAction === `member:${member.principal}`}
              disabled={view.saving}
              onclick={() => void controller.toggleMember(member)}
              >{member.enabled ? "Remove" : "Enable"}</UI.ActionButton
            >
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
            count={members.state.items.length}
            noun="members"
            previous={!!members.state.history.length && !members.state.busy}
            next={!!members.state.next && !members.state.busy}
            onprevious={() => void members.previous()}
            onnext={() => void members.next()}
          />
        {/snippet}
      </UI.ManagementSection>

      <UI.ManagementSection title="Bots" loading={agents.state.busy}>
        {#snippet actions()}<UI.Button
            size="sm"
            variant="outline"
            disabled={view.saving}
            onclick={() => addMembership("bots")}>Add bot</UI.Button
          >{/snippet}
        {#snippet toolbar()}
          <UI.SearchToolbar
            framed={false}
            label="Search bots"
            placeholder="Search bots…"
            bind:value={agents.state.search}
            loading={agents.state.busy}
            onsearch={() => void agents.search()}
          >
            {#snippet filters()}
              <UI.Select
                aria-label="Bot membership status"
                bind:value={agents.state.status}
                onchange={() => void agents.search()}
              >
                <UI.Option value="">All memberships</UI.Option><UI.Option
                  value="enabled">Enabled</UI.Option
                ><UI.Option value="disabled">Disabled</UI.Option>
              </UI.Select>
            {/snippet}
          </UI.SearchToolbar>
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
          href={(agent) =>
            `/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings/${encodeURIComponent(agent.agent)}`}
          emptyMessage={agents.state.loading
            ? "Loading bots…"
            : agents.state.error
              ? "Bots could not be loaded."
              : "No bot memberships match your search."}
        >
          {#snippet details(agent)}@{agent.alias}{/snippet}
          {#snippet badges(agent)}{#if !agent.enabled}<UI.Badge
                >Membership disabled</UI.Badge
              >{/if}{#if !agent.agent_enabled}<UI.Badge>Bot disabled</UI.Badge
              >{/if}{/snippet}
          {#snippet actions(agent)}
            <UI.ActionButton
              size="sm"
              variant="ghost"
              aria-label={`${agent.enabled ? "Remove" : "Enable"} membership for ${agent.label}`}
              pending={view.pendingAction === `bot:${agent.agent}`}
              disabled={view.saving}
              onclick={() => void controller.toggleAgent(agent)}
              >{agent.enabled ? "Remove" : "Enable"}</UI.ActionButton
            >
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
            count={agents.state.items.length}
            noun="bots"
            previous={!!agents.state.history.length && !agents.state.busy}
            next={!!agents.state.next && !agents.state.busy}
            onprevious={() => void agents.previous()}
            onnext={() => void agents.next()}
          />
        {/snippet}
      </UI.ManagementSection>
    </UI.ManagementLayout>
    {#if dialog === "details"}<SystemGroupDetailsDialog
        {controller}
        onClose={() => (dialog = null)}
      />
    {:else if dialog === "members" || dialog === "bots"}<SystemGroupMembershipDialog
        {controller}
        {workspaceID}
        {groupID}
        kind={dialog}
        onClose={() => (dialog = null)}
      />
    {/if}
  {/if}
</SystemWorkspaceFrame>
