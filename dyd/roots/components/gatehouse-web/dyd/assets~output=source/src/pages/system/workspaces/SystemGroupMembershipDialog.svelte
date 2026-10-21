<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    type SystemPrincipal,
    type SystemWorkspaceAgent,
  } from "../../../app/system"
  import {
    fetchAdminGroupMemberOptions,
    groupAdministration,
    groupAdminPath,
    type GroupScope,
  } from "../../../app/group-administration"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import type { SystemGroupController } from "./system-group-controller.svelte"
  let {
    controller,
    workspaceID,
    groupID,
    kind,
    onClose,
    scope = "system",
  }: {
    controller: SystemGroupController
    workspaceID: string
    groupID: string
    kind: "members" | "bots"
    onClose: () => void
    scope?: GroupScope
  } = $props()
  let view = $derived(controller.state)
  const runtime = useRuntime()
  let selected = $state("")
  let noun = $derived(kind === "members" ? "member" : "bot")
  const directory = createDirectoryController<{
    id: string
    label: string
    alias?: string
    disabled: boolean
  }>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: (next) => {
      if (scope === "system") runtime.access.setSystemAccess(next)
    },
    async fetchPage(name, cursor, _status, signal) {
      const response =
        kind === "members"
          ? await fetchAdminGroupMemberOptions(
              workspaceID,
              groupID,
              name,
              cursor,
              signal,
              scope,
            )
          : await groupAdministration(
              `${groupAdminPath(workspaceID, groupID, "/agent-options")}?${new URLSearchParams({ name, cursor, limit: "50" })}`,
              "GET",
              undefined,
              signal,
              scope,
            )
      if (!response.ok) return { response }
      const body = (await response.json()) as {
        principals?: SystemPrincipal[]
        agents?: SystemWorkspaceAgent[]
        next_cursor?: string
      }
      const items =
        kind === "members"
          ? (body.principals ?? []).map((person) => ({
              id: person.id,
              label: person.name ?? person.alias ?? "Unnamed person",
              alias: person.alias,
              disabled: !person.enabled,
            }))
          : (body.agents ?? []).map((bot) => ({
              id: bot.id,
              label: bot.label ?? bot.alias,
              alias: bot.alias,
              disabled: !bot.enabled,
            }))
      return { response, items, next: body.next_cursor }
    },
  })
  onMount(() => {
    void directory.search()
    return directory.stop
  })
  function close() {
    if (!view.saving) onClose()
  }
  async function save() {
    if (!selected || view.saving) return
    if (kind === "members") view.principal = selected
    else view.agent = selected
    if (
      kind === "members"
        ? await controller.addMember()
        : await controller.addAgent()
    )
      close()
  }
</script>

<UI.ResourceDialog
  open
  busy={view.saving}
  title={`Add ${noun}`}
  viewKey={kind}
  onClose={close}
>
  <UI.RecordList
    label={`Choose ${noun}`}
    selectable
    disabled={view.saving}
    bind:value={selected}
    items={directory.state.items}
    loading={directory.state.busy}
    scrollable={false}
    framed={false}
    emptyMessage={directory.state.loading
      ? `Loading ${noun}s…`
      : `No matching ${noun}s.`}
  >
    {#snippet toolbar()}
      <UI.SearchToolbar
        framed={false}
        label={`Search ${noun}s`}
        placeholder="Name or alias"
        bind:value={directory.state.search}
        loading={directory.state.busy}
        onsearch={() => void directory.search()}
      />
    {/snippet}
    {#snippet details(item)}{#if item.alias}@{item.alias}{/if}{/snippet}
  </UI.RecordList>
  <UI.PaginationFooter
    previous={!!directory.state.history.length}
    next={!!directory.state.next}
    loading={directory.state.busy}
    onprevious={() => void directory.previous()}
    onnext={() => void directory.next()}
  />
  {#if kind === "members"}
    <UI.Field>
      <UI.Label for="group-member-role">Role</UI.Label>
      <UI.Select
        id="group-member-role"
        bind:value={view.memberRole}
        disabled={view.saving}
      >
        <UI.Option value="member">Member</UI.Option>
        <UI.Option value="manager">Manager</UI.Option>
      </UI.Select>
      <UI.Text size="small" tone="muted"
        >Managers can edit the group and manage its people, roles, and bots.</UI.Text
      >
    </UI.Field>
  {/if}
  <UI.ActionFeedback
    error={view.feedback[kind].error || directory.state.error}
    pending={view.pendingAction === `${kind}:add`}
    label={`Adding ${noun}…`}
  />
  {#if directory.state.error}<UI.Row
      ><UI.Button
        variant="outline"
        size="sm"
        onclick={() => void directory.retry()}>Retry search</UI.Button
      ></UI.Row
    >{/if}
  <UI.ResourceActions includeClose={false}>
    <UI.Button variant="outline" disabled={view.saving} onclick={close}
      >Cancel</UI.Button
    >
    <UI.ActionButton
      pending={view.pendingAction === `${kind}:add`}
      disabled={view.saving ||
        !selected ||
        directory.state.busy ||
        !!directory.state.error}
      onclick={() => void save()}>Add {noun}</UI.ActionButton
    >
  </UI.ResourceActions>
</UI.ResourceDialog>
