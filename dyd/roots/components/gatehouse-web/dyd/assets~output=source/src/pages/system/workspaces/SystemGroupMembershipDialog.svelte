<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    searchSystemPrincipals,
    fetchSystemWorkspaceAgents,
    type SystemPrincipal,
    type SystemWorkspaceAgent,
  } from "../../../app/system"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import type { SystemGroupController } from "./system-group-controller.svelte"
  let {
    controller,
    workspaceID,
    kind,
    onClose,
  }: {
    controller: SystemGroupController
    workspaceID: string
    kind: "members" | "bots"
    onClose: () => void
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
    onSystemAccessChange: runtime.access.setSystemAccess,
    async fetchPage(name, cursor, _status, signal) {
      const response =
        kind === "members"
          ? await searchSystemPrincipals(name, cursor, "enabled", signal)
          : await fetchSystemWorkspaceAgents(workspaceID, name, cursor, signal)
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
              label: person.name ?? person.alias ?? person.id,
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
    {#snippet details(item)}{item.alias ?? item.id}{/snippet}
  </UI.RecordList>
  <UI.PaginationFooter
    count={directory.state.items.length}
    noun={`${noun}s`}
    previous={!!directory.state.history.length && !directory.state.busy}
    next={!!directory.state.next && !directory.state.busy}
    onprevious={() => void directory.previous()}
    onnext={() => void directory.next()}
  />
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
