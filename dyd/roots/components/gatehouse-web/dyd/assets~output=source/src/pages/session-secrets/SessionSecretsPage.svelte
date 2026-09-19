<script lang="ts">
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../app/activity"
  import PageBody from "../../components/PageBody.svelte"
  import PageHeading from "../../components/PageHeading.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
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
    <form
      class="stack"
      onsubmit={(event) => {
        event.preventDefault()
        void controller.save()
      }}
    >
      <PageHeading>
        <p class="eyebrow">Session Secret</p>
        <h2>{controller.state.creating ? "New Secret" : "Edit Secret"}</h2>
      </PageHeading>
      <div class="field">
        <label for="session-secret-description">Description</label
        >
        <textarea
          id="session-secret-description"
          autocomplete="off"
          rows="3"
          maxlength="4096"
          required
          bind:value={controller.state.description}></textarea>
      </div>
      <div class="field">
        <label for="session-secret-value"
          >{controller.state.creating ? "Value" : "New value (optional)"}</label
        >
        <textarea
          id="session-secret-value"
          autocomplete="new-password"
          rows="5"
          maxlength="1048576"
          required={controller.state.creating}
          bind:value={controller.state.value}></textarea>
        {#if !controller.state.creating}<p class="field-help">
            Leave blank to keep the current value.
          </p>{/if}
      </div>
      {#if controller.state.error !== ""}<p
          class="field-help"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </p>{/if}
      <div class="cluster">
        <button
          type="button"
          disabled={controller.state.saving}
          onclick={() => controller.cancelEdit()}>Cancel</button
        ><button
          class="primary"
          type="submit"
          disabled={controller.state.saving}
          >{controller.state.saving ? "Saving..." : "Save secret"}</button
        >
      </div>
    </form>
  {:else if controller.state.active !== null}
    <article class="stack">
      {#snippet secretActions()}<button
          class="small"
          type="button"
          onclick={() => controller.startEdit()}>Edit</button
        ><button
          class="secondary small"
          type="button"
          disabled={controller.state.deleting}
          onclick={() => void controller.remove()}
          >{controller.state.deleting ? "Removing..." : "Remove"}</button
        >{/snippet}
      <PageHeading as="header" actions={secretActions}>
        <p class="eyebrow">Session Secret</p>
        <h2>{controller.state.active.description}</h2>
        <small
          >By {controller.state.active.author.name ??
            controller.state.active.author.id} on {createdAtLabel(
            controller.state.active.created_at,
          )}{#if controller.state.active.updated_at !== controller.state.active.created_at}
            / Updated {createdAtLabel(
              controller.state.active.updated_at,
            )}{/if}</small
        >
      </PageHeading>
      {#if controller.state.error !== ""}<p
          class="field-help"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </p>{/if}
    </article>
  {:else}{#snippet collectionActions()}<button
        class="primary small"
        type="button"
        onclick={() => controller.startCreate()}>New secret</button
      >{/snippet}
    <PageHeading actions={collectionActions}>
      <h2>Session Secrets</h2>
    </PageHeading>
    <div class="list">
      {#if controller.state.status === "checking"}
        <p class="muted">Loading secrets...</p>
      {:else if controller.state.status === "unavailable"}
        <p class="muted">Secrets could not be loaded.</p>
      {:else}
        {#each controller.state.secrets as secret (secret.id)}
          <RouterLink
            class="list-item surface stack"
            href={secretPath(secret.id)}
            ><span class="stack"
              ><span>{secret.description}</span><small
                class="cluster muted"
                ><span>{secret.author.name ?? secret.author.id}</span><time
                  datetime={secret.updated_at}
                  >Updated {createdAtLabel(secret.updated_at)}</time
                ></small
              ></span
            ></RouterLink
          >
        {:else}<p class="muted">No secrets yet.</p>{/each}
      {/if}
    </div>
  {/if}
</PageBody>
