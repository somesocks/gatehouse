<script lang="ts">
  import { PageBody, PageHeading, RouterLink } from "../../ui"
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
  }: {
    workspaceID: string
    sessionID: string
    route: Route
    signal: AbortSignal
    activity: ActivityClient
    onAuthenticationLost: () => void
    onNavigate: (path: string, replace?: boolean) => void
    onBreadcrumbChange: (title: string | null) => void
  } = $props()
  const controller = untrack(() =>
    createSessionSecretsController({
      activity,
      onAuthenticationLost,
      onNavigate,
    }),
  )

  $effect(() => {
    const currentRoute = route
    if (
      currentRoute.kind === "session-secrets" ||
      currentRoute.kind === "session-secret-new" ||
      currentRoute.kind === "session-secret" ||
      currentRoute.kind === "session-secret-edit"
    ) {
      return untrack(() =>
        controller.start(workspaceID, sessionID, currentRoute, signal),
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

  function secretPath(secretID: string) {
    return `/app/wsp/${encodeURIComponent(workspaceID)}/ses/${encodeURIComponent(sessionID)}/secrets/${encodeURIComponent(secretID)}`
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
  {#if "secretID" in route && controller.state.detailStatus !== "ready"}
    <UI.AdminHeader title="Secret" />
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
  {:else if controller.state.editing}
    <UI.AdminHeader
      title={controller.state.creating ? "New secret" : "Edit secret"}
      description="Update the description and optionally replace the write-only value."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void controller.save()
      }}
    >
      <UI.FormSection
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
          {#if !controller.state.creating}<UI.Text as="p" size="small">
              Leave blank to keep the current value.
            </UI.Text>{/if}
        </UI.Field>
      </UI.FormSection>{#if controller.state.error !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </UI.Text>{/if}
      <UI.FormActions>
        <UI.Button
          variant="outline"
          type="button"
          disabled={controller.state.saving}
          onclick={() => controller.cancelEdit()}>Cancel</UI.Button
        ><UI.Button
          variant="default"
          type="submit"
          disabled={controller.state.saving}
          >{controller.state.saving ? "Saving..." : "Save secret"}</UI.Button
        >
      </UI.FormActions>
    </UI.Form>
  {:else if controller.state.active !== null}
    {#snippet secretActions()}<UI.Button
        variant="outline"
        size="sm"
        type="button"
        onclick={() => controller.startEdit()}>Edit</UI.Button
      ><UI.Button
        variant="outline"
        size="sm"
        type="button"
        disabled={controller.state.deleting}
        onclick={() => void controller.remove()}
        >{controller.state.deleting ? "Removing..." : "Remove"}</UI.Button
      >{/snippet}
    <UI.Stack as="article">
      <UI.AdminHeader
        kind="Chat secret"
        title="Secret"
        actions={secretActions}
      />
      <UI.DetailSection title="Secret information">
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
  {:else}{#snippet collectionActions()}<UI.Button
        variant="default"
        size="sm"
        type="button"
        onclick={() => controller.startCreate()}>New secret</UI.Button
      >{/snippet}
    <UI.AdminHeader
      title="Secrets"
      description="Manage descriptions and write-only secret values."
      actions={collectionActions}
    />
    <UI.SearchToolbar
      label="Search secrets"
      placeholder="Description"
      bind:value={controller.page.state.search}
      loading={controller.page.state.busy ||
        controller.state.status === "checking"}
      onsearch={() => void controller.page.search(controller.reload)}
    />
    <UI.RecordTable headers={["Description", "Author", "Updated"]}>
      {#if controller.state.status !== "ready"}<UI.TableEmpty
          columns={3}
          message={controller.state.status === "checking"
            ? "Loading secrets…"
            : "Secrets could not be loaded."}
        />
      {:else}{#each controller.state.secrets as secret (secret.id)}<UI.RecordRow
          >
            <UI.RecordCell primary
              ><RouterLink href={secretPath(secret.id)}
                >{secret.description}</RouterLink
              ></UI.RecordCell
            >
            <UI.RecordCell>{secret.author.name ?? "User"}</UI.RecordCell>
            <UI.RecordCell>{createdAtLabel(secret.updated_at)}</UI.RecordCell>
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={3}
            message="No secrets yet."
          />{/each}{/if}
    </UI.RecordTable>
    <UI.PaginationFooter
      noun="secrets"
      count={controller.state.secrets.length}
      previous={controller.page.state.history.length > 0}
      next={!!controller.page.state.next}
      loading={controller.page.state.busy ||
        controller.state.status === "checking"}
      onprevious={() => void controller.page.previous(controller.reload)}
      onnext={() => void controller.page.next(controller.reload)}
    />
    {#if controller.state.status === "unavailable"}<UI.Button
        variant="outline"
        onclick={() => void controller.reload()}>Try again</UI.Button
      >{/if}
  {/if}
</PageBody>
