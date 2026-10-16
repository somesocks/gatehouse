<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    fetchAdminGroupMembers,
    type SystemGroupMember,
  } from "../../../app/group-administration"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import PrincipalPicker from "../principals/PrincipalPicker.svelte"
  import SystemWorkspaceFrame from "./SystemWorkspaceFrame.svelte"
  import { createSystemGroupController } from "./system-group-controller.svelte"
  let { workspaceID, groupID }: { workspaceID: string; groupID: string } =
    $props()
  const runtime = useRuntime()
  const members = createDirectoryController<SystemGroupMember>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await fetchAdminGroupMembers(
        workspaceID,
        groupID,
        name,
        cursor,
        status,
        signal,
      )
      const result = response.ok
        ? ((await response.json()) as {
            members: SystemGroupMember[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.members, next: result?.next_cursor }
    },
  })
  const controller = createSystemGroupController({
    workspaceID: () => workspaceID,
    groupID: () => groupID,
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    onMembersChanged: () => members.reload(),
  })
  const state = controller.state
  onMount(() => {
    void controller.load()
    void members.search()
    return () => {
      controller.stop()
      members.stop()
    }
  })
</script>

<SystemWorkspaceFrame
  {workspaceID}
  breadcrumbs={[
    {
      label: "Groups",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}/groups`,
    },
    { label: state.group?.name ?? state.group?.alias ?? "Group" },
  ]}
>
  <UI.AdminContent>
    {#if state.loadError}<UI.Notice variant="destructive" role="alert"
        >{state.loadError}<UI.Button
          variant="outline"
          onclick={() => void controller.load(false)}>Retry</UI.Button
        ></UI.Notice
      >{/if}
    {#if state.error}<UI.Notice variant="destructive" role="alert"
        >{state.error}</UI.Notice
      >{/if}
    {#if state.notice}<UI.Text role="status">{state.notice}</UI.Text>{/if}
    {#if !state.group}<UI.Text
        >{state.loading
          ? "Loading group…"
          : "Group could not be loaded."}</UI.Text
      >{:else}
      <UI.AdminHeader
        kind="Group"
        title={state.group.name ?? state.group.alias ?? state.group.id}
        id={state.group.id}
        enabled={state.group.enabled}
      />
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          void controller.saveGroup()
        }}
      >
        <UI.FormSection title="Group settings">
          <UI.Text size="small" tone="muted"
            >Revision {state.group.revision}</UI.Text
          >
          <UI.Field
            ><UI.Label for="group-alias">Alias</UI.Label><UI.Input
              id="group-alias"
              required
              maxlength={256}
              bind:value={state.form.alias}
              disabled={state.saving}
            /><UI.Text size="small" tone="muted"
              >Unique in this workspace. Lowercase letters, numbers,
              underscores, and hyphens; start with a letter.</UI.Text
            ></UI.Field
          >
          <UI.Field
            ><UI.Label for="group-name">Display name</UI.Label><UI.Input
              id="group-name"
              maxlength={256}
              bind:value={state.form.name}
              disabled={state.saving}
            /></UI.Field
          >
          <UI.Label layout="choice"
            ><UI.Checkbox
              bind:checked={state.form.enabled}
              disabled={state.saving}
            />Enabled</UI.Label
          >
          <UI.Text size="small" tone="muted"
            >Disabling the group suspends access inherited through it.
            Memberships and grants are retained.</UI.Text
          >
        </UI.FormSection>
        <UI.FormActions
          >{#if state.groupConflict}<UI.Button
              type="button"
              variant="outline"
              onclick={() => void controller.load(true)}
              disabled={state.saving}>Reload group and replace draft</UI.Button
            >{/if}<UI.Button
            type="submit"
            disabled={state.saving || state.groupConflict}
            >{state.saving ? "Saving…" : "Save group"}</UI.Button
          ></UI.FormActions
        >
      </UI.Form>
      <UI.DetailSection title="Members">
        <UI.Form
          onsubmit={(event) => {
            event.preventDefault()
            void controller.addMember()
          }}
        >
          <PrincipalPicker bind:value={state.principal} />
          <UI.Button type="submit" disabled={state.saving || !state.principal}
            >Add member</UI.Button
          >
        </UI.Form>
        <UI.SearchToolbar
          label="Search members"
          bind:value={members.state.search}
          loading={members.state.busy}
          onsearch={() => void members.search()}
        />
        <UI.Field
          ><UI.Label for="members-status">Membership status</UI.Label><UI.Select
            id="members-status"
            bind:value={members.state.status}
            onchange={(event) => {
              members.state.status = event.currentTarget.value
              void members.search()
            }}
            ><UI.Option value="">All memberships</UI.Option><UI.Option
              value="enabled">Enabled</UI.Option
            ><UI.Option value="disabled">Disabled</UI.Option></UI.Select
          ></UI.Field
        >
        {#if members.state.error}<UI.Notice variant="destructive" role="alert"
            >{members.state.error}<UI.Button
              variant="outline"
              onclick={() => void members.retry()}>Retry</UI.Button
            ></UI.Notice
          >{/if}
        <UI.RecordTable
          headers={[
            "Principal",
            "Membership",
            "Account",
            "Revision",
            "Actions",
          ]}
        >
          {#each members.state.items as member (member.principal)}<UI.RecordRow
              ><UI.RecordCell primary
                ><UI.RouterLink
                  href={`/app/system/principals/${encodeURIComponent(member.principal)}`}
                  >{member.name ??
                    member.alias ??
                    member.principal}</UI.RouterLink
                ><UI.Identifier value={member.principal} /></UI.RecordCell
              ><UI.RecordCell label="Membership"
                ><UI.StatusBadge enabled={member.enabled} /></UI.RecordCell
              ><UI.RecordCell label="Account"
                ><UI.StatusBadge
                  enabled={member.principal_enabled}
                /></UI.RecordCell
              ><UI.RecordCell label="Revision">{member.revision}</UI.RecordCell
              ><UI.RecordCell actions
                ><UI.Button
                  variant="outline"
                  disabled={state.saving}
                  onclick={() => void controller.toggleMember(member)}
                  >{member.enabled
                    ? "Remove membership"
                    : "Enable membership"}</UI.Button
                ></UI.RecordCell
              ></UI.RecordRow
            >{:else}<UI.TableEmpty
              columns={5}
              message={members.state.busy
                ? "Loading members…"
                : "No memberships match your search."}
            />{/each}
        </UI.RecordTable>
        <UI.PaginationFooter
          count={members.state.items.length}
          previous={!!members.state.history.length}
          next={!!members.state.next}
          loading={members.state.busy}
          onprevious={() => void members.previous()}
          onnext={() => void members.next()}
        />
      </UI.DetailSection>
      <UI.DetailSection title="Workspace grants">
        <UI.Text size="small" tone="muted"
          >Each grant gives its role to this group's enabled members. Grants
          apply while the group and workspace are enabled.</UI.Text
        >
        <UI.Form
          layout="row"
          onsubmit={(event) => {
            event.preventDefault()
            void controller.createGrant()
          }}
          ><UI.Field
            ><UI.Label for="new-group-role">New grant role</UI.Label><UI.Select
              id="new-group-role"
              bind:value={state.newRole}
              disabled={state.saving}
              ><UI.Option value="member">Member</UI.Option><UI.Option
                value="contributor">Contributor</UI.Option
              ><UI.Option value="manager">Manager</UI.Option></UI.Select
            ></UI.Field
          ><UI.Button type="submit" disabled={state.saving}>Add grant</UI.Button
          ></UI.Form
        >
        {#if state.editingGrant}
          <UI.Form
            surface
            onsubmit={(event) => {
              event.preventDefault()
              void controller.saveGrant()
            }}
            ><UI.FormSection title="Edit workspace grant"
              ><UI.Identifier value={state.editingGrant.id} /><UI.Field
                ><UI.Label for="edit-group-role">Role</UI.Label><UI.Select
                  id="edit-group-role"
                  bind:value={state.grantRole}
                  disabled={state.saving}
                  ><UI.Option value="member">Member</UI.Option><UI.Option
                    value="contributor">Contributor</UI.Option
                  ><UI.Option value="manager">Manager</UI.Option></UI.Select
                ></UI.Field
              ><UI.Label layout="choice"
                ><UI.Checkbox
                  bind:checked={state.grantEnabled}
                  disabled={state.saving}
                />Enabled</UI.Label
              ></UI.FormSection
            ><UI.FormActions
              ><UI.Button
                type="button"
                variant="outline"
                disabled={state.saving}
                onclick={controller.cancelGrant}>Cancel</UI.Button
              ><UI.Button
                type="submit"
                disabled={state.saving || state.grantConflict}
                >Save grant</UI.Button
              ></UI.FormActions
            ></UI.Form
          >
        {/if}
        <UI.RecordTable
          headers={["Grant", "Role", "Status", "Revision", "Actions"]}
        >
          {#each state.grants as grant (grant.id)}<UI.RecordRow
              ><UI.RecordCell primary
                ><UI.Identifier value={grant.id} /></UI.RecordCell
              ><UI.RecordCell label="Role">{grant.role}</UI.RecordCell
              ><UI.RecordCell label="Status"
                ><UI.StatusBadge enabled={grant.enabled} /></UI.RecordCell
              ><UI.RecordCell label="Revision">{grant.revision}</UI.RecordCell
              ><UI.RecordCell actions
                ><UI.Row
                  ><UI.Button
                    variant="outline"
                    disabled={state.saving}
                    onclick={() => controller.beginGrant(grant)}
                    >Edit role</UI.Button
                  ><UI.Button
                    variant="outline"
                    disabled={state.saving}
                    onclick={() => void controller.toggleGrant(grant)}
                    >{grant.enabled
                      ? "Revoke grant"
                      : "Enable grant"}</UI.Button
                  ></UI.Row
                ></UI.RecordCell
              ></UI.RecordRow
            >{:else}<UI.TableEmpty
              columns={5}
              message="No workspace grants. Add a grant to give this group's members workspace access."
            />{/each}
        </UI.RecordTable>
      </UI.DetailSection>
    {/if}
  </UI.AdminContent>
</SystemWorkspaceFrame>
