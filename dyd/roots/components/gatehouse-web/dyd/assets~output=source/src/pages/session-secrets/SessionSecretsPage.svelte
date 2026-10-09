<script lang="ts">
  import { PageBody, PageHeading, RouterLink } from "../../ui"
  import * as UI from "../../ui"
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../app/activity"
  import type { Route } from "../../route"
  import { createSessionSecretsController } from "./session-secrets-controller.svelte"

  type SessionSecretsRoute = Extract<
    Route,
    { kind: "session-secrets" | "session-secret-new" | "session-secret" }
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
      currentRoute.kind === "session-secret"
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
  {#if controller.state.editing}
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void controller.save()
      }}
    >
      <PageHeading>
        <UI.Text as="p" size="eyebrow">Session Secret</UI.Text>
        <UI.Heading level={2}
          >{controller.state.creating
            ? "New Secret"
            : "Edit Secret"}</UI.Heading
        >
      </PageHeading>
      <UI.Field>
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
      <UI.Field>
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
      {#if controller.state.error !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </UI.Text>{/if}
      <UI.Row>
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
      </UI.Row>
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
      <PageHeading as="header" actions={secretActions}>
        <UI.Text as="p" size="eyebrow">Session Secret</UI.Text>
        <UI.Heading level={2}>{controller.state.active.description}</UI.Heading>
        <UI.Text as="small" size="small"
          >By {controller.state.active.author.name ??
            controller.state.active.author.id} on {createdAtLabel(
            controller.state.active.created_at,
          )}{#if controller.state.active.updated_at !== controller.state.active.created_at}
            / Updated {createdAtLabel(
              controller.state.active.updated_at,
            )}{/if}</UI.Text
        >
      </PageHeading>
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
    <PageHeading actions={collectionActions}>
      <UI.Heading level={2}>Session Secrets</UI.Heading>
    </PageHeading>
    <UI.List>
      {#if controller.state.status === "checking"}
        <UI.Text as="p" tone="muted">Loading secrets...</UI.Text>
      {:else if controller.state.status === "unavailable"}
        <UI.Text as="p" tone="muted">Secrets could not be loaded.</UI.Text>
      {:else}
        {#each controller.state.secrets as secret (secret.id)}
          <RouterLink variant="item" href={secretPath(secret.id)}
            ><UI.Stack as="span"
              ><UI.Text>{secret.description}</UI.Text><UI.Row
                tone="muted"
                as="small"
                ><UI.Text>{secret.author.name ?? secret.author.id}</UI.Text
                ><UI.Text as="time" datetime={secret.updated_at}
                  >Updated {createdAtLabel(secret.updated_at)}</UI.Text
                ></UI.Row
              ></UI.Stack
            ></RouterLink
          >
        {:else}<UI.Text as="p" tone="muted">No secrets yet.</UI.Text>{/each}
      {/if}
    </UI.List>
  {/if}
</PageBody>
