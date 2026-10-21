<script lang="ts">
  import * as UI from "../../../ui"
  import type { SystemGroupController } from "./system-group-controller.svelte"
  let {
    controller,
    onClose,
  }: { controller: SystemGroupController; onClose: () => void } = $props()
  let state = $derived(controller.state)
  const uid = $props.id()
  function close() {
    if (!state.saving && !state.reloading) onClose()
  }
  async function save() {
    if (await controller.saveGroup()) onClose()
  }
  async function reload() {
    await controller.load(true)
    if (!state.loadError) controller.clearFeedback("details")
  }
</script>

<UI.ResourceDialog
  open
  compact
  busy={state.saving || state.reloading}
  title="Edit group details"
  viewKey="details"
  onClose={close}
>
  <UI.Form
    id={uid}
    onsubmit={(event) => {
      event.preventDefault()
      void save()
    }}
  >
    <UI.FormSection title="Group details" dialog>
      <UI.Field>
        <UI.Label for={`${uid}-alias`}>Alias</UI.Label>
        <UI.Input
          id={`${uid}-alias`}
          required
          maxlength={256}
          bind:value={state.form.alias}
          disabled={state.saving || state.reloading}
        />
        <UI.Text size="small" tone="muted"
          >Unique in this workspace. Lowercase letters, numbers, underscores,
          and hyphens; start with a letter.</UI.Text
        >
      </UI.Field>
      <UI.Field>
        <UI.Label for={`${uid}-name`}>Display name</UI.Label>
        <UI.Input
          id={`${uid}-name`}
          maxlength={256}
          bind:value={state.form.name}
          disabled={state.saving || state.reloading}
        />
      </UI.Field>
    </UI.FormSection>
    <UI.ActionFeedback
      error={state.feedback.details.error || state.loadError}
      pending={state.pendingAction === "details:save"}
      label="Saving group details…"
    />
    {#if state.groupConflict}
      <UI.Row
        ><UI.ActionButton
          variant="outline"
          pending={state.reloading}
          disabled={state.saving}
          onclick={() => void reload()}
          >Reload details and replace draft</UI.ActionButton
        ></UI.Row
      >
    {/if}
  </UI.Form>
  <UI.ResourceActions includeClose={false}>
    <UI.Button
      variant="outline"
      disabled={state.saving || state.reloading}
      onclick={close}>Cancel</UI.Button
    >
    <UI.ActionButton
      type="submit"
      form={uid}
      pending={state.pendingAction === "details:save"}
      disabled={state.saving ||
        state.reloading ||
        state.groupConflict ||
        !state.dirty}>Save details</UI.ActionButton
    >
  </UI.ResourceActions>
</UI.ResourceDialog>
