<script lang="ts">
  import * as UI from "../ui"
  import {
    participantRoles,
    type SessionParticipantGrant,
  } from "../app/session-participants"

  let {
    grant,
    name,
    canManage,
    saving,
    onEdit,
    onRemove,
  }: {
    grant: SessionParticipantGrant
    name: string
    canManage: boolean
    saving: boolean
    onEdit: () => void
    onRemove: () => void
  } = $props()
</script>

<UI.Row justify="end" wrap={false}>
  <UI.Text tone="muted"
    >{participantRoles.find((role) => role.value === grant.role)
      ?.label}</UI.Text
  >
  {#if canManage}
    <UI.Button
      variant="ghost"
      size="icon-sm"
      disabled={saving}
      aria-label={`Edit role for ${name}`}
      title="Edit role"
      onclick={onEdit}><UI.Pencil aria-hidden="true" /></UI.Button
    >
    <UI.Button
      variant="ghost"
      size="icon-sm"
      disabled={saving}
      aria-label={`Remove ${name} from chat`}
      title="Remove"
      onclick={onRemove}><UI.Trash2 aria-hidden="true" /></UI.Button
    >
  {/if}
</UI.Row>
