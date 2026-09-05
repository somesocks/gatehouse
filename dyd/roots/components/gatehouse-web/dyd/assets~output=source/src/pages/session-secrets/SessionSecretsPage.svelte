<script lang="ts">
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../app/activity"
  import type { Workspace } from "../../app/access"
  import type { Route } from "../../route"
  import { createSessionSecretsController } from "./session-secrets-controller.svelte"

  type SessionSecretsRoute = Extract<Route, { kind: "session-secrets" | "session-secret-new" | "session-secret" }>
  let { workspace, session, route, activity, onAuthenticationLost, onNavigate, onBreadcrumbChange }: {
    workspace: Workspace
    session: { id: string }
    route: Route
    activity: ActivityClient
    onAuthenticationLost: () => void
    onNavigate: (path: string, replace?: boolean) => void
    onBreadcrumbChange: (title: string | null) => void
  } = $props()
  const controller = untrack(() => createSessionSecretsController({ activity, onAuthenticationLost, onNavigate }))

  $effect(() => {
    const workspaceID = workspace.id
    const sessionID = session.id
    const currentRoute = route
    if (currentRoute.kind === "session-secrets" || currentRoute.kind === "session-secret-new" || currentRoute.kind === "session-secret") {
      return untrack(() => controller.start(workspaceID, sessionID, currentRoute))
    }
  })

  $effect(() => {
    onBreadcrumbChange(controller.state.creating ? "New Secret" : controller.state.active?.description ?? null)
    return () => onBreadcrumbChange(null)
  })

  function secretPath(secretID: string) {
    return `/app/wsp/${encodeURIComponent(workspace.id)}/ses/${encodeURIComponent(session.id)}/secrets/${encodeURIComponent(secretID)}`
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

<section class="project-note-page">
  {#if controller.state.editing}
    <form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void controller.save() }}>
      <div class="project-note-page-heading"><div><p class="eyebrow">Session Secret</p><h2>{controller.state.creating ? "New Secret" : "Edit Secret"}</h2></div></div>
      <div class="field"><label class="label" for="session-secret-description">Description</label><div class="control"><textarea class="textarea" id="session-secret-description" autocomplete="off" rows="3" maxlength="4096" required bind:value={controller.state.description}></textarea></div></div>
      <div class="field"><label class="label" for="session-secret-value">{controller.state.creating ? "Value" : "New value (optional)"}</label><div class="control"><textarea class="textarea" id="session-secret-value" autocomplete="new-password" rows="5" maxlength="1048576" required={controller.state.creating} bind:value={controller.state.value}></textarea></div>{#if !controller.state.creating}<p class="help">Leave blank to keep the current value.</p>{/if}</div>
      {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
      <div class="project-note-actions"><button class="button" type="button" disabled={controller.state.saving} onclick={() => controller.cancelEdit()}>Cancel</button><button class="button is-primary" type="submit" disabled={controller.state.saving}>{controller.state.saving ? "Saving..." : "Save secret"}</button></div>
    </form>
  {:else if controller.state.active !== null}
    <article class="project-note-view">
      <header class="project-note-page-heading"><div><p class="eyebrow">Session Secret</p><h2>{controller.state.active.description}</h2><small>By {controller.state.active.author.name ?? controller.state.active.author.id} on {createdAtLabel(controller.state.active.created_at)}{#if controller.state.active.updated_at !== controller.state.active.created_at} / Updated {createdAtLabel(controller.state.active.updated_at)}{/if}</small></div><div class="project-note-actions"><button class="button is-small" type="button" onclick={() => controller.startEdit()}>Edit</button><button class="button is-small is-danger is-light" type="button" disabled={controller.state.deleting} onclick={() => void controller.remove()}>{controller.state.deleting ? "Removing..." : "Remove"}</button></div></header>
      {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    </article>
  {:else}
    <div class="collection-heading"><h2>Session Secrets</h2><button class="button is-primary is-small" type="button" onclick={() => controller.startCreate()}>New secret</button></div>
    <div class="collection-list">
      {#if controller.state.status === "checking"}
        <p class="dashboard-empty">Loading secrets...</p>
      {:else if controller.state.status === "unavailable"}
        <p class="dashboard-empty">Secrets could not be loaded.</p>
      {:else}
        {#each controller.state.secrets as secret (secret.id)}
          <a class="dashboard-row project-note-row" href={secretPath(secret.id)} onclick={(event) => { event.preventDefault(); onNavigate(secretPath(secret.id)) }}><span class="dashboard-row-content"><span class="project-note-title">{secret.description}</span><span class="dashboard-row-meta"><span>{secret.author.name ?? secret.author.id}</span><time datetime={secret.updated_at}>Updated {createdAtLabel(secret.updated_at)}</time></span></span></a>
        {:else}<p class="dashboard-empty">No secrets yet.</p>{/each}
      {/if}
    </div>
  {/if}
</section>
