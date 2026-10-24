<script lang="ts">
  import { untrack } from "svelte"
  import { reducesRoleAccess } from "../app/role-access"
  import * as UI from "../ui"
  import {
    participantRoles,
    participantNoun,
    type ParticipantKind,
    type ParticipantRole,
    type SessionParticipantGrant,
  } from "../app/session-participants"

  const uid = $props.id()
  let {
    mode,
    kind,
    grant,
    name,
    multiple = false,
    canManage,
    saving,
    error,
    onUpdate,
    onRemove,
    onReload,
    onClose,
  }: {
    mode: "role" | "remove"
    kind: ParticipantKind
    grant: SessionParticipantGrant
    name: string
    multiple?: boolean
    canManage: boolean
    saving: boolean
    error: string
    onUpdate: (
      grant: SessionParticipantGrant,
      role: ParticipantRole,
    ) => Promise<boolean>
    onRemove: (grant: SessionParticipantGrant) => Promise<boolean>
    onReload: () => Promise<SessionParticipantGrant | undefined>
    onClose: (saved?: boolean) => void
  } = $props()
  // Freeze the grant and revision on opening. Background roster updates must not
  // overwrite the draft or silently advance the revision used by confirmation.
  let snapshot = $state<SessionParticipantGrant>(untrack(() => ({ ...grant })))
  let draft = $state<ParticipantRole>(untrack(() => grant.role))
  const reducing = $derived(mode === "role" && reducesRoleAccess(snapshot.role, draft))
  let reloading = $state(false)
  let active = true
  $effect(() => () => {
    active = false
  })

  async function save() {
    if (saving || reloading || !canManage) return
    const saved =
      mode === "role"
        ? await onUpdate(snapshot, draft)
        : await onRemove(snapshot)
    if (saved && active) onClose(true)
  }
  async function reload() {
    if (saving || reloading) return
    reloading = true
    try {
      const latest = await onReload()
      if (!active || !latest) return
      snapshot = { ...latest }
      draft = latest.role
    } finally {
      if (active) reloading = false
    }
  }
</script>

<UI.Dialog.Root
  open={true}
  onOpenChange={(open) => {
    if (!open && !saving && !reloading) onClose()
  }}
>
  <UI.Dialog.Content
    showCloseButton={false}
    onEscapeKeydown={(event) => { if (saving || reloading) event.preventDefault() }}
    onInteractOutside={(event) => { if (saving || reloading) event.preventDefault() }}
  >
    <UI.Dialog.Title
      >{mode === "role"
        ? `Edit role for ${name}`
        : `Remove ${participantNoun(kind)}`}</UI.Dialog.Title
    >
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void save()
      }}
    >
      {#if mode === "role"}
        {#if reducing}<UI.Dialog.Description>
          This change reduces {name}'s permissions from this grant. Other direct and group grants still apply.
        </UI.Dialog.Description>{/if}
        <UI.Field
          ><UI.Label for={`${uid}-role`}>Role</UI.Label>
          <UI.Select
            id={`${uid}-role`}
            bind:value={draft}
            disabled={saving || reloading || !canManage}
          >
            {#each participantRoles as role}<UI.Option value={role.value}
                >{role.label}</UI.Option
              >{/each}
          </UI.Select>
        </UI.Field>
      {:else}
        <UI.Dialog.Description
          >{multiple
            ? `Remove ${participantRoles.find((role) => role.value === snapshot.role)?.label.toLowerCase()} access for ${name} from this chat?`
            : `Remove ${name} from this chat?`}</UI.Dialog.Description
        >
      {/if}
      {#if error}<UI.Notice variant="destructive" role="alert"
          >{error}<UI.Button
            variant="outline"
            size="sm"
            disabled={saving || reloading}
            onclick={() => void reload()}>Reload</UI.Button
          ></UI.Notice
        >{/if}
      <UI.Row justify="end">
        <UI.Button variant="outline" disabled={saving || reloading} onclick={() => onClose()}>Cancel</UI.Button
        >
        <UI.Button
          type="submit"
          variant={mode === "remove" || reducing ? "destructive" : "default"}
          disabled={saving ||
            reloading ||
            !canManage ||
            (mode === "role" && draft === snapshot.role)}
          >{saving
            ? mode === "role"
              ? "Saving…"
              : "Removing…"
            : mode === "role"
              ? "Save"
              : "Remove"}</UI.Button
        >
      </UI.Row>
    </UI.Form>
  </UI.Dialog.Content>
</UI.Dialog.Root>
