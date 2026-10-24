<script lang="ts">
  import { onDestroy, untrack } from "svelte"
  import * as UI from "../../ui"
  import type { GroupRole, SystemGroupMember } from "../../app/group-administration"
  import type { SystemGroupController } from "../system/workspaces/system-group-controller.svelte"

  let { controller, member, onReload, onClose }: {
    controller: SystemGroupController
    member: SystemGroupMember
    onReload: () => Promise<SystemGroupMember | undefined>
    onClose: () => void
  } = $props()
  const uid = $props.id()
  const view = $derived(controller.state)
  let snapshot = $state(untrack(() => ({ ...member })))
  let role = $state<GroupRole>(untrack(() => member.role))
  const name = $derived(snapshot.name ?? snapshot.alias ?? "Unnamed person")
  const reducing = $derived(snapshot.role === "admin" && role === "member")
  let reloading = $state(false)
  let reloadError = $state("")
  let active = true
  onDestroy(() => {
    active = false
    controller.cancelMutation()
  })

  function close() {
    if (!view.saving) onClose()
  }
  async function save() {
    if (view.saving || reloading || role === snapshot.role) return
    if (await controller.changeMemberRole(snapshot, role)) {
      if (active) onClose()
    }
  }
  async function reload() {
    if (view.saving || reloading) return
    reloading = true
    reloadError = ""
    try {
      const latest = await onReload()
      if (!active) return
      if (!latest) {
        reloadError = "The member could not be reloaded. Close this dialog and search for them again."
        return
      }
      snapshot = { ...latest }
      role = latest.role
      controller.clearFeedback("members")
    } finally {
      if (active) reloading = false
    }
  }
</script>

<UI.ResourceDialog
  open
  compact
  includeClose={false}
  busy={view.saving}
  title={`Edit role for ${name}`}
  viewKey="member-role"
  onClose={close}
>
  <UI.Form id={uid} onsubmit={(event) => { event.preventDefault(); void save() }}>
    <UI.Field>
      <UI.Label for={`${uid}-role`}>Role</UI.Label>
      <UI.Select id={`${uid}-role`} bind:value={role} disabled={view.saving || reloading}>
        <UI.Option value="member">Member</UI.Option>
        <UI.Option value="admin">Admin</UI.Option>
      </UI.Select>
      <UI.Text size="small" tone="muted">Admins can edit the group and manage its members, bots, and secrets.</UI.Text>
    </UI.Field>
    {#if reducing}
      <UI.Text as="p">{name} will remain a member but will no longer be able to manage this group.</UI.Text>
    {/if}
    <UI.ActionFeedback
      error={reloadError || view.feedback.members.error}
      pending={reloading}
      label="Reloading member…"
    />
    {#if reloadError || view.feedback.members.error}
      <UI.Button variant="outline" disabled={view.saving || reloading} onclick={() => void reload()}>Reload role and replace draft</UI.Button>
    {/if}
  </UI.Form>
  {#snippet footer()}
    <UI.Button variant="outline" disabled={view.saving} onclick={close}>Cancel</UI.Button>
    <UI.ActionButton
      type="submit"
      form={uid}
      variant={reducing ? "destructive" : "default"}
      pending={view.saving}
      disabled={reloading || role === snapshot.role}
    >Save role</UI.ActionButton>
  {/snippet}
</UI.ResourceDialog>
