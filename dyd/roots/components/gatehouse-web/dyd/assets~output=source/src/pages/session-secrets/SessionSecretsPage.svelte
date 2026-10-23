<script lang="ts">
  import { PageBody } from "../../ui"
  import * as UI from "../../ui"
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../app/activity"
  import type { Route } from "../../route"
  import { createSessionSecretsController } from "./session-secrets-controller.svelte"

  type SessionSecretsRoute = Extract<
    Route,
    {
      kind:
        | "session-secrets"
        | "session-secret-new"
        | "session-secret"
        | "session-secret-edit"
    }
  >
  let {
    workspaceID,
    sessionID,
    route,
    signal,
    activity,
    onAuthenticationLost,
    onNavigate,
    onBreadcrumbChange,
    embedded = false,
  }: {
    workspaceID: string
    sessionID: string
    route: Route
    signal: AbortSignal
    activity: ActivityClient
    onAuthenticationLost: () => void
    onNavigate: (path: string, replace?: boolean) => void
    onBreadcrumbChange: (title: string | null) => void
    embedded?: boolean
  } = $props()
  const controller = untrack(() =>
    createSessionSecretsController({
      activity,
      onAuthenticationLost,
      onNavigate,
    }),
  )
  let removal = $state<{ id: string; description: string } | null>(null)

  $effect(() => {
    const currentRoute = route
    const workspace = workspaceID
    const session = sessionID
    const requestSignal = signal
    if (
      currentRoute.kind === "session-secrets" ||
      currentRoute.kind === "session-secret-new" ||
      currentRoute.kind === "session-secret" ||
      currentRoute.kind === "session-secret-edit"
    ) {
      removal = null
      return untrack(() =>
        controller.start(workspace, session, currentRoute, requestSignal),
      )
    }
  })

  $effect(() => {
    onBreadcrumbChange(
      controller.state.creating
        ? "New Secret"
        : (controller.state.active?.description ?? null),
    )
    return () => onBreadcrumbChange(null)
  })

  const secretsPath = $derived(
    `/app/wsp/${encodeURIComponent(workspaceID)}/ses/${encodeURIComponent(sessionID)}/secrets`,
  )
  function secretPath(secretID: string) {
    return `${secretsPath}/${encodeURIComponent(secretID)}`
  }

  function requestRemove() {
    if (!controller.state.active || controller.state.deleting) return
    controller.state.error = ""
    removal = { ...controller.state.active }
  }

  function confirmRemove() {
    if (removal && controller.state.active?.id === removal.id) void controller.remove()
  }

  function createdAtLabel(value: string) {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) {
      return value
    }
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }
</script>

<PageBody fluid>
  {#if controller.state.reloading}<UI.Text role="status"
      >Refreshing secrets…</UI.Text
    >{/if}
  {#if controller.state.reloadError}<UI.Notice
      variant="destructive"
      role="alert"
      >{controller.state.reloadError}<UI.Button
        variant="outline"
        onclick={() => void controller.reload()}>Retry</UI.Button
      ></UI.Notice
    >{/if}
  {#if "secretID" in route && controller.state.detailStatus !== "ready"}
    <UI.AdminHeader title="Secret" compact={embedded} />
    {#if controller.state.detailStatus === "checking"}<UI.Text role="status"
        >Loading secret…</UI.Text
      >{:else}<UI.Notice variant="destructive" role="alert"
        >{controller.state.error}</UI.Notice
      ><UI.Button
        variant="outline"
        onclick={() =>
          "secretID" in route &&
          controller.start(
            workspaceID,
            sessionID,
            route as SessionSecretsRoute,
            signal,
          )}>Try again</UI.Button
      >{/if}
  {:else if removal !== null}
    <UI.ResourceRemoval noun="secret" name={removal.description} busy={controller.state.deleting} error={controller.state.error} onCancel={() => { removal = null; controller.state.error = "" }} onConfirm={confirmRemove} />
  {:else if controller.state.editing}
    <UI.AdminHeader
      compact={embedded}
      title={controller.state.creating ? "New secret" : "Edit secret"}
      description={embedded ? undefined : "Update the description and optionally replace the write-only value."}
    />
    <UI.Form
      id="session-secret-form"
      surface={!embedded}
      compact={embedded}
      onsubmit={(event) => {
        event.preventDefault()
        void controller.save()
      }}
    >
      <UI.FormSection
        dialog={embedded}
        title="Description"
        description="Identify the secret without including its value."
        ><UI.Field>
          <UI.Label for="session-secret-description">Description</UI.Label>
          <UI.Textarea
            id="session-secret-description"
            autocomplete="off"
            rows={3}
            maxlength={4096}
            required
            bind:value={controller.state.description}
          ></UI.Textarea>
        </UI.Field>
      </UI.FormSection><UI.FormSection
        dialog={embedded}
        title="Secret value"
        description={controller.state.creating
          ? "Values are write-only and cannot be displayed later."
          : "Leave the replacement blank to keep the current value."}
        ><UI.Field>
          <UI.Label for="session-secret-value"
            >{controller.state.creating
              ? "Value"
              : "New value (optional)"}</UI.Label
          >
          <UI.Textarea
            id="session-secret-value"
            autocomplete="new-password"
            rows={5}
            maxlength={1048576}
            required={controller.state.creating}
            bind:value={controller.state.value}
          ></UI.Textarea>
          <UI.Text as="p" size="small">
            {controller.state.creating ? "Values are write-only and cannot be displayed later." : "Leave blank to keep the current value."}
          </UI.Text>
        </UI.Field>
      </UI.FormSection>{#if controller.state.error !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </UI.Text>{/if}
      <UI.FormActions dialog={embedded}>
        <UI.RouterLink
          variant="outline"
          disabled={controller.state.saving}
          href={controller.state.creating || controller.state.active === null
            ? secretsPath
            : secretPath(controller.state.active.id)}>Cancel</UI.RouterLink
        ><UI.Button
          variant="default"
          type="submit"
          form="session-secret-form"
          disabled={controller.state.saving}
          >{controller.state.saving ? "Saving..." : "Save secret"}</UI.Button
        >
      </UI.FormActions>
    </UI.Form>
  {:else if controller.state.active !== null}
    {@const secretID = controller.state.active.id}
    {#snippet secretActions()}<UI.Button
        variant={embedded ? "destructive" : "outline"}
        size={embedded ? "default" : "sm"}
        type="button"
        disabled={controller.state.deleting}
        onclick={requestRemove}
        >{controller.state.deleting ? "Removing..." : "Remove"}</UI.Button
      ><UI.RouterLink variant={embedded ? "default" : "outline"} size={embedded ? "default" : "sm"} href={`${secretPath(secretID)}/edit`}>Edit</UI.RouterLink>{/snippet}
    <UI.Stack as="article">
      {#if !embedded}<UI.AdminHeader
        kind="Chat secret"
        title="Secret"
        actions={secretActions}
      />{/if}
      {#if embedded}<UI.ResourceActions>{@render secretActions()}</UI.ResourceActions>{/if}
      <UI.DetailSection title="Secret information" compact={embedded}>
        <UI.Property label="Description"
          >{controller.state.active.description}</UI.Property
        >
        <UI.Property label="Author"
          >{controller.state.active.author.name ?? "User"}</UI.Property
        >
        <UI.Property label="Created"
          >{createdAtLabel(controller.state.active.created_at)}</UI.Property
        >
        <UI.Property label="Updated"
          >{createdAtLabel(controller.state.active.updated_at)}</UI.Property
        >
        <UI.Property label="Value"
          >Write-only; edit to replace the stored value.</UI.Property
        >
      </UI.DetailSection>
      {#if controller.state.error !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </UI.Text>{/if}
    </UI.Stack>
  {:else}{#snippet collectionActions()}<UI.RouterLink
        variant="default"
        size={embedded ? "default" : "sm"}
        href={`${secretsPath}/new`}>New secret</UI.RouterLink
      >{/snippet}
    {#if embedded}
      <UI.ResourceActions>{@render collectionActions()}</UI.ResourceActions>
    {:else}
      <UI.AdminHeader
        title="Secrets"
        description="Manage descriptions and write-only secret values."
        actions={collectionActions}
      />
    {/if}
    <UI.RecordList
      label="Secrets"
      items={controller.state.secrets.map((secret) => ({ ...secret, label: secret.description }))}
      href={(secret) => secretPath(secret.id)}
      loading={controller.page.state.busy || controller.state.busy}
      scrollable={false}
      pageKey={JSON.stringify([controller.page.state.query, controller.page.state.cursor])}
      emptyMessage={controller.state.status === "checking" ? "Loading…" : controller.state.status === "unavailable" ? "Secrets could not be loaded." : "No matching secrets."}
    >
      {#snippet toolbar()}<UI.SearchToolbar framed={false} label="Search secrets" placeholder="Search" bind:value={controller.page.state.search} loading={controller.page.state.busy || controller.state.busy} onsearch={() => void controller.page.search(controller.reload)} />{/snippet}
      {#snippet details(secret)}{secret.author.name ?? "User"} · {createdAtLabel(secret.updated_at)}{/snippet}
    </UI.RecordList>
    {#if controller.page.state.history.length > 0 || controller.page.state.next}<UI.PaginationFooter
      showCount={false}
      count={controller.state.secrets.length}
      previous={controller.page.state.history.length > 0}
      next={!!controller.page.state.next}
      loading={controller.page.state.busy ||
        controller.state.status === "checking"}
      onprevious={() => void controller.page.previous(controller.reload)}
      onnext={() => void controller.page.next(controller.reload)}
    />{/if}
    {#if controller.state.status === "unavailable"}<UI.Button
        variant="outline"
        onclick={() => void controller.reload()}>Try again</UI.Button
      >{/if}
  {/if}
</PageBody>
