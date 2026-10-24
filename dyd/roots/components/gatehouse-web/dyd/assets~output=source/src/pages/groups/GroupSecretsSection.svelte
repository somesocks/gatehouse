<script lang="ts">
  import { onMount, untrack } from "svelte"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import { createGroupSecretsController } from "./group-secrets-controller.svelte"
  import { routePath } from "../../route"

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
  const route = $derived(runtime.state.route.kind === "group" && runtime.state.route.workspaceID === workspaceID && runtime.state.route.groupID === groupID ? runtime.state.route : null)
  const secretDialog = $derived(route?.dialog === "secret-create" || route?.dialog === "secret-edit" ? route.dialog : null)
  const dialogPath = (dialog: "secret-create" | "secret-edit", secretID?: string) => routePath({ kind: "group", workspaceID, groupID, dialog, secretID })
  $effect(() => {
    const dialog = secretDialog
    const id = route?.secretID
    const ready = view.active && view.canManage
    return untrack(() => {
      if (!ready) return
      if (dialog === "secret-create") controller.begin("create")
      else if (dialog === "secret-edit" && id) controller.beginEdit(id)
      else if (view.dialog === "create" || view.dialog === "edit") controller.cancelDialog()
      if (dialog) return controller.cancelDialog
    })
  })
  function closeDialog() {
    if (view.saving) return
    controller.cancelDialog()
    if (secretDialog) runtime.navigate(routePath({ kind: "group", workspaceID, groupID }), true)
  }
  async function save() {
    const routed = secretDialog !== null
    const openingRoute = route
    if (await controller.save()) {
      if (routed && view.active && route === openingRoute) runtime.navigate(routePath({ kind: "group", workspaceID, groupID }), true)
    }
  }
</script>

<UI.ManagementSection title="Secrets" loading={directory.state.busy}>
  {#snippet actions()}
    {#if view.canManage}
      <UI.RouterLink variant="default" disabled={view.saving} href={dialogPath("secret-create")}>Add secret</UI.RouterLink>
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
        <UI.Row justify="end">
        <UI.RouterLink size="sm" variant="ghost" aria-label={`Edit ${secret.description}`} title="Edit secret" disabled={view.saving} href={dialogPath("secret-edit", secret.id)}><UI.Pencil aria-hidden="true" /></UI.RouterLink>
        <UI.Button size="icon-sm" variant="ghost" aria-label={`Remove ${secret.description}`} title="Remove secret" disabled={view.saving} onclick={() => controller.begin("remove", secret)}><UI.Trash2 aria-hidden="true" /></UI.Button>
        </UI.Row>
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
  <UI.ResourceDialog
    open
    compact
    includeClose={false}
    busy={view.saving}
    title={view.dialog === "create" ? "Add secret" : view.dialog === "edit" ? "Edit secret" : "Remove secret"}
    viewKey={view.dialog}
    onClose={closeDialog}
  >
      <UI.Form id={`${uid}-form`} onsubmit={(event) => { event.preventDefault(); void save() }}>
        {#if view.reloading}<UI.Text role="status">Loading secret…</UI.Text>{/if}
        {#if view.dialog === "remove"}
          <UI.Text as="p">Remove “{view.snapshot?.description}”? The stored credential will be deleted and bots will no longer be able to use it through this group.</UI.Text>
        {:else}
          <UI.Text as="p" size="small" tone="muted">Secret values are write-only. Stored values cannot be viewed.</UI.Text>
          <UI.Field>
            <UI.Label for={`${uid}-description`}>Description</UI.Label>
            <UI.Input id={`${uid}-description`} maxlength={4096} required bind:value={view.description} disabled={view.saving || view.reloading || !view.canManage} />
          </UI.Field>
          <UI.Field>
            <UI.Label for={`${uid}-value`}>{view.dialog === "edit" ? "Replacement value (optional)" : "Value"}</UI.Label>
            <UI.Textarea id={`${uid}-value`} aria-describedby={view.dialog === "edit" ? `${uid}-value-help` : undefined} rows={5} maxlength={1048576} required={view.dialog === "create"} bind:value={view.value} disabled={view.saving || view.reloading || !view.canManage} />
            {#if view.dialog === "edit"}<UI.Text id={`${uid}-value-help`} size="small" tone="muted">Leave blank to keep the stored value.</UI.Text>{/if}
          </UI.Field>
        {/if}
        {#if view.error}
          <UI.Notice variant="destructive" role="alert">
            {view.error}
            {#if view.snapshot || route?.secretID}<UI.Button size="sm" variant="outline" disabled={view.saving || view.reloading} onclick={() => void controller.reloadSecret(route?.secretID)}>Reload secret and replace draft</UI.Button>{/if}
          </UI.Notice>
        {/if}
      </UI.Form>
      {#snippet footer()}
          <UI.Button variant="outline" disabled={view.saving} onclick={closeDialog}>Cancel</UI.Button>
          <UI.ActionButton type="submit" form={`${uid}-form`} pending={view.saving} variant={view.dialog === "remove" ? "destructive" : "default"} disabled={view.reloading || !view.canManage || (view.dialog !== "create" && !view.snapshot) || (view.dialog !== "remove" && (!view.description.trim() || (view.dialog === "create" && !view.value)))}>
            {view.dialog === "remove" ? "Remove secret" : view.dialog === "create" ? "Add secret" : "Save secret"}
          </UI.ActionButton>
      {/snippet}
  </UI.ResourceDialog>
{/if}
