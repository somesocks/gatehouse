<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import { createGroupSecretsController } from "./group-secrets-controller.svelte"

  let { workspaceID, groupID }: { workspaceID: string; groupID: string } = $props()
  const runtime = useRuntime()
  const controller = createGroupSecretsController({
    activity: runtime.activity,
    onAuthenticationLost: runtime.requireLogin,
    onAccessLost: () => runtime.navigate(`/app/wsp/${encodeURIComponent(workspaceID)}/grp`, true),
  })
  const directory = controller.directory
  const view = controller.state
  const uid = $props.id()
  onMount(() => controller.start(workspaceID, groupID))
</script>

<UI.ManagementSection title="Secrets" loading={directory.state.busy}>
  {#snippet actions()}
    {#if view.canManage}
      <UI.Button size="sm" variant="outline" disabled={view.saving} onclick={() => controller.begin("create")}>Add secret</UI.Button>
    {/if}
  {/snippet}
  {#snippet toolbar()}
    <UI.SearchToolbar framed={false} label="Search secrets" placeholder="Search secrets…" bind:value={directory.state.search} loading={directory.state.busy} onsearch={() => void directory.search()} />
  {/snippet}
  <UI.RecordList
    label="Group secrets"
    framed={false}
    scrollable={false}
    loading={directory.state.busy}
    items={directory.state.items.map((secret) => ({ ...secret, label: secret.description }))}
    emptyMessage={directory.state.loading ? "Loading secrets…" : directory.state.hasLoaded ? "No matching secrets." : "Secrets could not be loaded."}
  >
    {#snippet actions(secret)}
      {#if view.canManage}
        <UI.Button size="icon-sm" variant="ghost" aria-label={`Edit ${secret.description}`} title="Edit secret" disabled={view.saving} onclick={() => controller.begin("edit", secret)}><UI.Pencil aria-hidden="true" /></UI.Button>
        <UI.Button size="icon-sm" variant="ghost" aria-label={`Remove ${secret.description}`} title="Remove secret" disabled={view.saving} onclick={() => controller.begin("remove", secret)}><UI.Trash2 aria-hidden="true" /></UI.Button>
      {/if}
    {/snippet}
  </UI.RecordList>
  {#snippet footer()}
    <UI.ActionFeedback error={directory.state.error || (view.dialog === null ? view.error : "")} pending={directory.state.busy} label="Refreshing secrets…" />
    {#if directory.state.error || (view.dialog === null && view.error)}
      <UI.Row><UI.Button size="sm" variant="outline" onclick={() => void directory.retry()}>Reload secrets</UI.Button></UI.Row>
    {/if}
    <UI.PaginationFooter showCount={false} previous={directory.state.history.length > 0} next={!!directory.state.next} loading={directory.state.busy} onprevious={() => void directory.previous()} onnext={() => void directory.next()} />
  {/snippet}
</UI.ManagementSection>

{#if view.dialog !== null}
  <UI.Dialog.Root open={true} onOpenChange={(open) => { if (!open) controller.cancelDialog() }}>
    <UI.Dialog.Content showCloseButton>
      <UI.Dialog.Title>{view.dialog === "create" ? "Add secret" : view.dialog === "edit" ? "Edit secret" : "Remove secret"}</UI.Dialog.Title>
      <UI.Form onsubmit={(event) => { event.preventDefault(); void controller.save() }}>
        {#if view.dialog === "remove"}
          <UI.Dialog.Description>Remove {view.snapshot?.description}?</UI.Dialog.Description>
        {:else}
          <UI.Field>
            <UI.Label for={`${uid}-description`}>Description</UI.Label>
            <UI.Input id={`${uid}-description`} maxlength={4096} required bind:value={view.description} disabled={view.saving || view.reloading || !view.canManage} />
          </UI.Field>
          <UI.Field>
            <UI.Label for={`${uid}-value`}>{view.dialog === "edit" ? "Replacement value" : "Value"}</UI.Label>
            <UI.Textarea id={`${uid}-value`} rows={5} maxlength={1048576} required={view.dialog === "create"} bind:value={view.value} disabled={view.saving || view.reloading || !view.canManage} />
          </UI.Field>
        {/if}
        {#if view.error}
          <UI.Notice variant="destructive" role="alert">
            {view.error}
            {#if view.snapshot}<UI.Button size="sm" variant="outline" disabled={view.saving || view.reloading} onclick={() => void controller.reloadSecret()}>Reload secret</UI.Button>{/if}
          </UI.Notice>
        {/if}
        <UI.Row justify="end">
          <UI.Button variant="outline" onclick={controller.cancelDialog}>Cancel</UI.Button>
          <UI.Button type="submit" variant={view.dialog === "remove" ? "destructive" : "default"} disabled={view.saving || view.reloading || !view.canManage || (view.dialog !== "remove" && (!view.description.trim() || (view.dialog === "create" && !view.value)))}>
            {view.saving ? "Saving…" : view.dialog === "remove" ? "Remove" : "Save"}
          </UI.Button>
        </UI.Row>
      </UI.Form>
    </UI.Dialog.Content>
  </UI.Dialog.Root>
{/if}
