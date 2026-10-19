<script lang="ts">
  import * as UI from "../../../ui"
  import type { SystemGroupController } from "./system-group-controller.svelte"
  let {
    controller,
    editing = false,
    onClose,
  }: {
    controller: SystemGroupController
    editing?: boolean
    onClose: () => void
  } = $props()
  let state = $derived(controller.state)
  const uid = $props.id()
  function close() {
    if (!state.saving) {
      controller.cancelGrant()
      onClose()
    }
  }
  async function save() {
    if (editing ? await controller.saveGrant() : await controller.createGrant())
      close()
  }
</script>

{#snippet roles()}
  <UI.Option value="member">Member</UI.Option>
  <UI.Option value="contributor">Contributor</UI.Option>
  <UI.Option value="manager">Manager</UI.Option>
{/snippet}

<UI.ResourceDialog
  open
  compact
  busy={state.saving}
  title={editing ? "Edit workspace grant" : "Add workspace grant"}
  viewKey="grant"
  onClose={close}
>
  <UI.Form
    id={uid}
    onsubmit={(event) => {
      event.preventDefault()
      void save()
    }}
  >
    <UI.Text tone="muted" size="small"
      >This role applies to the group's enabled human members while the group
      and workspace are enabled.</UI.Text
    >
    <UI.FormSection title="Workspace grant" dialog>
      <UI.Field>
        <UI.Label for={`${uid}-role`}>Role</UI.Label>
        {#if editing}
          <UI.Select
            id={`${uid}-role`}
            bind:value={state.grantRole}
            disabled={state.saving}
          >
            {@render roles()}
          </UI.Select>
        {:else}
          <UI.Select
            id={`${uid}-role`}
            bind:value={state.newRole}
            disabled={state.saving}
          >
            {@render roles()}
          </UI.Select>
        {/if}
      </UI.Field>
      {#if editing}
        <UI.Label layout="choice"
          ><UI.Checkbox
            bind:checked={state.grantEnabled}
            disabled={state.saving}
          />Enabled</UI.Label
        >
        {#if state.editingGrant}
          <UI.Row tone="muted"
            ><UI.Identifier value={state.editingGrant.id} /><UI.Text
              size="small">Revision {state.editingGrant.revision}</UI.Text
            ></UI.Row
          >
        {/if}
      {/if}
    </UI.FormSection>
    <UI.ActionFeedback
      error={state.feedback.grants.error}
      pending={state.pendingAction.startsWith("grant")}
      label="Saving workspace grant…"
    />
  </UI.Form>
  <UI.ResourceActions includeClose={false}>
    <UI.Button variant="outline" disabled={state.saving} onclick={close}
      >Cancel</UI.Button
    >
    <UI.ActionButton
      type="submit"
      form={uid}
      pending={state.pendingAction.startsWith("grant")}
      disabled={state.saving || (editing && state.grantConflict)}
      >{editing ? "Save grant" : "Add grant"}</UI.ActionButton
    >
  </UI.ResourceActions>
</UI.ResourceDialog>
