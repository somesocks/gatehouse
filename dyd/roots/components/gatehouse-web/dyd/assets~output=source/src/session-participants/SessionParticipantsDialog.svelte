<script lang="ts">
  import * as UI from "../ui"
  import {
    participantName,
    participantNoun,
    participantPlural,
    type ParticipantKind,
    type SessionParticipantGrant,
  } from "../app/session-participants"
  import type { SessionParticipantsController } from "./session-participants-controller.svelte"
  import SessionParticipantAddDialog from "./SessionParticipantAddDialog.svelte"
  import SessionParticipantActionDialog from "./SessionParticipantActionDialog.svelte"
  import SessionParticipantRole from "./SessionParticipantRole.svelte"

  let {
    workspaceID,
    sessionID,
    controller,
    modal = $bindable(),
  }: {
    workspaceID: string
    sessionID: string
    controller: SessionParticipantsController
    modal?: UI.DialogHandle
  } = $props()
  let tab = $state("principal")
  let adding = $state<ParticipantKind | null>(null)
  let action = $state<{
    mode: "role" | "remove"
    kind: ParticipantKind
    grant: SessionParticipantGrant
    name: string
    multiple: boolean
  } | null>(null)
  const tabs = [
    { value: "principal", label: "People" },
    { value: "group", label: "Groups" },
    { value: "agent", label: "Bots" },
  ]

  function close() {
    adding = null
    action = null
    controller.close()
  }
  function closeAction(saved = false) {
    controller.cancelMutation()
    if (!saved) controller.state.error = ""
    action = null
  }
  async function reloadGrant() {
    const target = action
    if (!target) return
    controller.state.error = ""
    if (!(await controller.reload())) {
      if (action === target)
        controller.state.error = "The participant could not be refreshed."
      return
    }
    if (action !== target) return
    const directory =
      controller.directory(target.kind)
    const latest = directory.state.items
      .flatMap((item) => item.grants)
      .find((grant) => grant.id === target.grant.id)
    if (!latest)
      controller.state.error =
        "This participant is no longer on this page. Close this dialog and refresh the list."
    return latest
  }
  async function added() {
    controller.state.error = ""
    if (!(await controller.reload()))
      controller.state.error =
        "The participant was added, but the list could not be refreshed. Try again."
  }
</script>

<UI.Modal bind:modal title="Participants" size="wide" onClose={close}>
  {#if controller.state.open}
    {#if controller.state.error && action === null}<UI.Notice
        variant="destructive"
        role="alert"
        >{controller.state.error}<UI.Button
          variant="outline"
          size="sm"
          onclick={() => void controller.reload()}>Reload</UI.Button
        ></UI.Notice
      >{/if}
    <UI.Tabs label="Participant type" items={tabs} bind:value={tab}>
      {#snippet children(value)}
        {@const kind = value as ParticipantKind}
        {@const directory =
          controller.directory(kind)}
        {@const noun = participantPlural(kind)}
        <UI.Stack>
          {#if directory.state.error}<UI.Notice
              variant="destructive"
              role="alert"
              >{directory.state.error}<UI.Button
                size="sm"
                variant="outline"
                onclick={() => void directory.retry()}>Try again</UI.Button
              ></UI.Notice
            >{/if}
          <UI.RecordList
            label={noun}
            loading={directory.state.busy}
            items={directory.state.items.map((item) => ({
              ...item,
              label: participantName(item, kind),
              disabled: !item.enabled,
            }))}
            emptyMessage={directory.state.loading
              ? "Loading…"
              : !directory.state.hasLoaded
                ? "Participants could not be loaded."
                : `No matching ${noun}.`}
          >
            {#snippet toolbar()}<UI.SearchToolbar
                framed={false}
                label={`Search ${noun}`}
                placeholder="Search"
                bind:value={directory.state.search}
                loading={directory.state.busy}
                onsearch={() => void directory.search()}
              />{/snippet}
            {#snippet actions(item)}
              <UI.Stack>
                {#each item.grants as grant (grant.id)}<SessionParticipantRole
                    {grant}
                    name={participantName(item, kind)}
                    canManage={controller.state.canManage}
                    saving={controller.state.saving}
                    onEdit={() => {
                      controller.state.error = ""
                      action = {
                        mode: "role",
                        kind,
                        grant: { ...grant },
                        name: participantName(item, kind),
                        multiple: item.grants.length > 1,
                      }
                    }}
                    onRemove={() => {
                      controller.state.error = ""
                      action = {
                        mode: "remove",
                        kind,
                        grant: { ...grant },
                        name: participantName(item, kind),
                        multiple: item.grants.length > 1,
                      }
                    }}
                  />{/each}
              </UI.Stack>
            {/snippet}
          </UI.RecordList>
          {#if directory.state.history.length > 0 || directory.state.next}
            <UI.PaginationFooter
              showCount={false}
              count={directory.state.items.length}
              previous={directory.state.history.length > 0}
              next={!!directory.state.next}
              loading={directory.state.busy}
              onprevious={() => void directory.previous()}
              onnext={() => void directory.next()}
            />
          {/if}
        </UI.Stack>
      {/snippet}
    </UI.Tabs>
    <UI.Row justify="end">
      <UI.Button variant="outline" onclick={() => modal?.close()}
        >Close</UI.Button
      >
      {#if controller.state.canManage}<UI.Button
          disabled={controller.state.saving}
          onclick={() => {
            adding = tab as ParticipantKind
          }}>Add {participantNoun(tab as ParticipantKind)}</UI.Button
        >{/if}
    </UI.Row>
    {#if adding !== null}
      <SessionParticipantAddDialog
        {workspaceID}
        {sessionID}
        kind={adding}
        canManage={controller.state.canManage}
        onClose={() => {
          adding = null
        }}
        onAdded={added}
        onPermissionsChanged={() => {
          void controller.reload()
        }}
      />
    {/if}
    {#if action !== null}
      <SessionParticipantActionDialog
        mode={action.mode}
        kind={action.kind}
        grant={action.grant}
        name={action.name}
        multiple={action.multiple}
        canManage={controller.state.canManage}
        saving={controller.state.saving}
        error={controller.state.error}
        onUpdate={controller.update}
        onRemove={controller.remove}
        onReload={reloadGrant}
        onClose={closeAction}
      />
    {/if}
  {/if}
</UI.Modal>
