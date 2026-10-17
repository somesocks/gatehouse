<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../ui"
  import { useRuntime } from "../app/runtime.svelte"
  import {
    participantName,
    participantRoles,
    type ParticipantKind,
  } from "../app/session-participants"
  import { createSessionParticipantAddController } from "./session-participant-add-controller.svelte"

  let {
    workspaceID,
    sessionID,
    kind,
    canManage,
    onClose,
    onAdded,
    onPermissionsChanged,
  }: {
    workspaceID: string
    sessionID: string
    kind: ParticipantKind
    canManage: boolean
    onClose: () => void
    onAdded: () => Promise<void>
    onPermissionsChanged: () => void
  } = $props()
  const runtime = useRuntime()
  const controller = createSessionParticipantAddController({
    onAuthenticationLost: runtime.requireLogin,
    onAdded: () => onAdded(),
    onPermissionsChanged: () => onPermissionsChanged(),
  })
  const uid = $props.id()
  const label = $derived(kind === "principal" ? "person" : "group")
  onMount(() => controller.open(workspaceID, sessionID, kind))
  async function save() {
    if (canManage && (await controller.save())) onClose()
  }
</script>

<UI.Dialog.Root
  open={true}
  onOpenChange={(open) => {
    if (!open) onClose()
  }}
>
  <UI.Dialog.Content showCloseButton size="wide">
    <UI.Dialog.Title>Add {label}</UI.Dialog.Title>
    {#if controller.options.state.error}<UI.Notice
        variant="destructive"
        role="alert"
        >{controller.options.state.error}<UI.Button
          size="sm"
          variant="outline"
          onclick={() => void controller.options.retry()}>Try again</UI.Button
        ></UI.Notice
      >{/if}
    <UI.RecordList
      label={`Choose ${label}`}
      selectable
      value={controller.state.subject}
      onSelect={controller.select}
      disabled={controller.state.saving || !canManage}
      loading={controller.options.state.busy}
      items={controller.options.state.items.map((item) => ({
        id: item.id,
        label: participantName(item, kind),
      }))}
      emptyMessage={controller.options.state.loading
        ? "Loading…"
        : controller.options.state.hasLoaded
          ? `No matching ${kind === "principal" ? "people" : "groups"}.`
          : "Options could not be loaded."}
    >
      {#snippet toolbar()}<UI.SearchToolbar
          framed={false}
          label={`Search ${kind === "principal" ? "people" : "groups"}`}
          placeholder="Search"
          bind:value={controller.options.state.search}
          loading={controller.options.state.busy || controller.state.saving}
          onsearch={() => void controller.options.search()}
        />{/snippet}
    </UI.RecordList>
    {#if controller.options.state.history.length > 0 || controller.options.state.next}<UI.PaginationFooter
        showCount={false}
        noun={kind === "principal" ? "people" : "groups"}
        count={controller.options.state.items.length}
        previous={controller.options.state.history.length > 0}
        next={!!controller.options.state.next}
        loading={controller.options.state.busy || controller.state.saving}
        onprevious={() => void controller.options.previous()}
        onnext={() => void controller.options.next()}
      />{/if}
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void save()
      }}
    >
      {#if controller.state.selectedName && !controller.options.state.items.some((item) => item.id === controller.state.subject)}<UI.Text
          tone="muted">Selected: {controller.state.selectedName}</UI.Text
        >{/if}
      <UI.Field>
        <UI.Label for={`${uid}-role`}>Role</UI.Label>
        <UI.Select
          id={`${uid}-role`}
          bind:value={controller.state.role}
          disabled={controller.state.saving || !canManage}
        >
          {#each participantRoles as role}<UI.Option value={role.value}
              >{role.label}</UI.Option
            >{/each}
        </UI.Select>
      </UI.Field>
      {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
          >{controller.state.error}</UI.Notice
        >{/if}
      <UI.Row justify="end">
        <UI.Button variant="outline" onclick={onClose}>Cancel</UI.Button>
        <UI.Button
          type="submit"
          disabled={controller.state.saving ||
            !controller.state.subject ||
            !canManage}
          >{controller.state.saving ? "Adding…" : `Add ${label}`}</UI.Button
        >
      </UI.Row>
    </UI.Form>
  </UI.Dialog.Content>
</UI.Dialog.Root>
