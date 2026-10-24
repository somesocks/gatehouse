<script lang="ts">
  import * as UI from "../../ui"
  import { projectMemberName, projectRoles } from "../../app/project-members"
  import { reducesRoleAccess } from "../../app/role-access"
  import { roleLabel, roleDescription } from "../../app/roles"
  import type { ProjectAccessController } from "./project-access-controller.svelte"

  let { controller }: {
    controller: ProjectAccessController
  } = $props()
  const uid = $props.id()
</script>

{#if controller.state.open}
    {#if controller.state.adding}
      {@const kind = controller.state.adding}
      {@const options = controller.options(kind)}
      {@const noun = kind === "principal" ? "person" : "group"}
      <UI.Dialog.Root open={true} onOpenChange={(open) => { if (!open) controller.cancelAction() }}>
        <UI.Dialog.Content
          size="wide"
          showCloseButton={!controller.state.saving}
          onEscapeKeydown={(event) => { if (controller.state.saving) event.preventDefault() }}
          onInteractOutside={(event) => { if (controller.state.saving) event.preventDefault() }}
        >
          <UI.Dialog.Title>Add {noun}</UI.Dialog.Title>
          {#if options.state.error}<UI.Notice variant="destructive" role="alert">{options.state.error}<UI.Button variant="outline" onclick={() => void options.retry()}>Try again</UI.Button></UI.Notice>{/if}
          <UI.RecordList label={`Choose ${noun}`} selectable value={controller.state.subject} onSelect={controller.select} disabled={controller.state.saving || !controller.state.canManage} loading={options.state.busy} items={options.state.items.map((item) => ({ id: item.id, label: projectMemberName(item) }))} emptyMessage={options.state.loading ? "Loading…" : options.state.hasLoaded ? "No matching recipients." : "Options could not be loaded."}>
            {#snippet toolbar()}<UI.SearchToolbar framed={false} label={`Search ${kind === "principal" ? "people" : "groups"}`} placeholder="Search" bind:value={options.state.search} loading={options.state.busy} onsearch={() => void options.search()} />{/snippet}
          </UI.RecordList>
          {#if options.state.history.length > 0 || options.state.next}<UI.PaginationFooter showCount={false} count={options.state.items.length} previous={options.state.history.length > 0} next={!!options.state.next} loading={options.state.busy} onprevious={() => void options.previous()} onnext={() => void options.next()} />{/if}
          <UI.Form onsubmit={(event) => { event.preventDefault(); void controller.add() }}>
            {#if controller.state.selectedName}<UI.Text tone="muted">Selected: {controller.state.selectedName}</UI.Text>{/if}
            <UI.Field>
              <UI.Label for={`${uid}-add-role`}>Role</UI.Label>
              <UI.Select id={`${uid}-add-role`} bind:value={controller.state.role} disabled={controller.state.saving || !controller.state.canManage}>
                {#each projectRoles as role}<UI.Option value={role.value}>{role.label}</UI.Option>{/each}
              </UI.Select>
              <UI.Text tone="muted" size="small">{roleDescription(controller.state.role)}</UI.Text>
            </UI.Field>
            {#if controller.state.error}<UI.Notice variant="destructive" role="alert">{controller.state.error}</UI.Notice>{/if}
            <UI.Row justify="end">
              <UI.Button variant="outline" disabled={controller.state.saving} onclick={controller.cancelAction}>Cancel</UI.Button>
              <UI.Button type="submit" disabled={controller.state.saving || !controller.state.subject || !controller.state.canManage}>{controller.state.saving ? "Adding…" : `Add ${noun}`}</UI.Button>
            </UI.Row>
          </UI.Form>
        </UI.Dialog.Content>
      </UI.Dialog.Root>
    {/if}
    {#if controller.state.action}
       {@const action = controller.state.action}
       {@const reducing = action.mode === "role" && reducesRoleAccess(action.grant.role, action.role)}
      <UI.Dialog.Root open={true} onOpenChange={(open) => { if (!open) controller.cancelAction() }}>
        <UI.Dialog.Content
          showCloseButton={!controller.state.saving && !controller.state.reloadingAction}
          onEscapeKeydown={(event) => { if (controller.state.saving || controller.state.reloadingAction) event.preventDefault() }}
          onInteractOutside={(event) => { if (controller.state.saving || controller.state.reloadingAction) event.preventDefault() }}
        >
          <UI.Dialog.Title>{action.mode === "role" ? `Edit role for ${action.name}` : "Remove project grant"}</UI.Dialog.Title>
          <UI.Form onsubmit={(event) => { event.preventDefault(); void controller.saveAction() }}>
             {#if action.mode === "role"}
               {#if reducing}<UI.Dialog.Description>
                 This change reduces {action.name}'s permissions from this grant. Other direct and group grants still apply.
               </UI.Dialog.Description>{/if}
              <UI.Field>
                <UI.Label for={`${uid}-edit-role`}>Role</UI.Label>
                <UI.Select id={`${uid}-edit-role`} bind:value={action.role} disabled={controller.state.saving || controller.state.reloadingAction || !controller.state.canManage}>
                  {#each projectRoles as role}<UI.Option value={role.value}>{role.label}</UI.Option>{/each}
                </UI.Select>
                <UI.Text tone="muted" size="small">{roleDescription(action.role)}</UI.Text>
              </UI.Field>
            {:else}
              <UI.Dialog.Description>Remove the {roleLabel(action.grant.role)} grant for {action.name}? Other direct or group grants will continue to apply.</UI.Dialog.Description>
            {/if}
            {#if controller.state.error}<UI.Notice variant="destructive" role="alert">{controller.state.error}<UI.Button variant="outline" disabled={controller.state.saving || controller.state.reloadingAction} onclick={() => void controller.reloadAction()}>Reload grant</UI.Button></UI.Notice>{/if}
            <UI.Row justify="end">
              <UI.Button variant="outline" disabled={controller.state.saving || controller.state.reloadingAction} onclick={controller.cancelAction}>Cancel</UI.Button>
              <UI.Button type="submit" variant={action.mode === "remove" || reducing ? "destructive" : "default"} disabled={controller.state.saving || controller.state.reloadingAction || !controller.state.canManage || (action.mode === "role" && action.role === action.grant.role)}>{controller.state.saving ? "Saving…" : action.mode === "role" ? "Save" : "Remove grant"}</UI.Button>
            </UI.Row>
          </UI.Form>
        </UI.Dialog.Content>
      </UI.Dialog.Root>
    {/if}
{/if}
