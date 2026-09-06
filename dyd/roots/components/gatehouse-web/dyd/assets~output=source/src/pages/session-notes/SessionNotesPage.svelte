<script lang="ts">
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../app/activity"
  import { sessionNotesAPIPath, type SessionNote } from "../../app/session-notes"
  import type { Workspace } from "../../app/access"
  import { renderMarkdown } from "../../markdown"
  import type { Route } from "../../route"
  import { createSessionNotesController } from "./session-notes-controller.svelte"

  type SessionNotesRoute = Extract<Route, { kind: "session-notes" | "session-note-new" | "session-note" | "session-note-edit" | "session-note-revision" }>
  let { workspace, session, route, activity, onAuthenticationLost, onNavigate, onBreadcrumbChange, onResetHistory, onOpenHistory }: { workspace: Workspace; session: { id: string }; route: Route; activity: ActivityClient; onAuthenticationLost: () => void; onNavigate: (path: string, replace?: boolean) => void; onBreadcrumbChange: (title: string | null) => void; onResetHistory: () => void; onOpenHistory: (notesPath: string, noteID: string, onRevision: (revision: number) => void) => void } = $props()
  const controller = untrack(() => createSessionNotesController({ activity, onAuthenticationLost, onNavigate, onResetHistory }))
  $effect(() => {
    const currentRoute = route
    if (currentRoute.kind === "session-notes" || currentRoute.kind === "session-note-new" || currentRoute.kind === "session-note" || currentRoute.kind === "session-note-edit" || currentRoute.kind === "session-note-revision") return untrack(() => controller.start(workspace.id, session.id, currentRoute))
  })
  $effect(() => { onBreadcrumbChange(controller.state.creating ? "New Note" : controller.state.active?.title ?? null); return () => onBreadcrumbChange(null) })
  const notePath = (id: string) => `/app/wsp/${encodeURIComponent(workspace.id)}/ses/${encodeURIComponent(session.id)}/notes/${encodeURIComponent(id)}`
  function authorLabel(author: SessionNote["author"]) { if (author.principal !== undefined) return author.principal.name ?? author.principal.id; if (author.agent !== undefined) return author.agent.label ?? author.agent.id; return author.gateway ?? "Unknown" }
  function dateLabel(value: string) { const date = new Date(value); if (Number.isNaN(date.getTime())) return value; const number = (part: number) => part.toString().padStart(2, "0"); return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}` }
</script>

<section class="project-note-page">
  {#if controller.state.editing}
    <form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void controller.save() }}>
      <div class="project-note-page-heading"><div><p class="eyebrow">Session Note</p><h2>{controller.state.creating ? "New Note" : "Edit Note"}</h2></div></div>
      <div class="field"><label class="label" for="session-note-title">Title</label><div class="control"><input class="input" id="session-note-title" autocomplete="off" maxlength="256" required bind:value={controller.state.title} /></div></div>
      <div class="field"><label class="label" for="session-note-description">Description (optional)</label><div class="control"><textarea class="textarea" id="session-note-description" autocomplete="off" rows="3" maxlength="4096" bind:value={controller.state.description}></textarea></div></div>
      <div class="field"><label class="label" for="session-note-body">Content (optional)</label><div class="control"><textarea class="textarea project-note-body-input" id="session-note-body" autocomplete="off" rows="18" maxlength="1048576" bind:value={controller.state.body}></textarea></div></div>
      <div class="field"><label class="checkbox"><input type="checkbox" autocomplete="off" bind:checked={controller.state.sensitive} /> Sensitive: content is marked sensitive when agents read it.</label></div>
      {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
      <div class="project-note-actions"><button class="button" type="button" disabled={controller.state.saving} onclick={() => controller.cancelEdit()}>Cancel</button><button class="button is-primary" type="submit" disabled={controller.state.saving}>{controller.state.saving ? "Saving..." : "Save note"}</button></div>
    </form>
  {:else if controller.state.pageStatus === "checking" && route.kind !== "session-notes"}
    <p class="dashboard-empty">Loading note...</p>
  {:else if controller.state.pageStatus === "not-found"}
    <p class="dashboard-empty">Note not found.</p>
  {:else if controller.state.pageStatus === "unavailable"}
    <p class="dashboard-empty">Note could not be loaded.</p>
  {:else if controller.state.active !== null}
    {@const note = controller.state.active}
    {@const displayed = controller.state.revision ?? note}
    <article class="project-note-view">
      <header class="project-note-page-heading"><div><p class="eyebrow">{controller.state.revision === null ? "Session Note" : `Session Note Revision ${controller.state.revision.revision}`}</p><h2><span class="project-note-title">{displayed.title}{#if displayed.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span></h2>{#if displayed.description !== ""}<p>{displayed.description}</p>{/if}<small>By {authorLabel(displayed.author)} on {dateLabel(displayed.created_at)}</small></div><div class="project-note-actions">{#if controller.state.revision === null}<button class="button is-small" type="button" onclick={() => controller.startEdit()}>Edit</button>{:else}<button class="button is-small" type="button" onclick={() => controller.showCurrentRevision()}>Current revision</button>{/if}<button class="button is-small" type="button" onclick={() => onOpenHistory(sessionNotesAPIPath(workspace.id, session.id), note.id, controller.openRevision)} >History</button>{#if controller.state.revision === null}<button class="button is-small is-danger is-light" type="button" disabled={controller.state.deleting} onclick={() => void controller.remove()}>{controller.state.deleting ? "Removing..." : "Remove"}</button>{/if}</div></header>
      {#if displayed.body !== undefined && displayed.body !== ""}<div class="markdown-content project-note-markdown">{@html renderMarkdown(displayed.body)}</div>{/if}
      {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    </article>
  {:else}
    <div class="collection-heading"><h2>Session Notes</h2><button class="button is-primary is-small" type="button" onclick={() => controller.startCreate()}>New note</button></div>
    <div class="collection-list">{#if controller.state.status === "checking"}<p class="dashboard-empty">Loading notes...</p>{:else if controller.state.status === "unavailable"}<p class="dashboard-empty">Notes could not be loaded.</p>{:else}{#each controller.state.notes as note (note.id)}<a class="dashboard-row project-note-row" href={notePath(note.id)} onclick={(event) => { event.preventDefault(); onNavigate(notePath(note.id)) }}><span class="dashboard-row-content"><span class="project-note-title">{note.title}{#if note.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if note.description !== ""}<span class="project-note-description">{note.description}</span>{/if}<span class="dashboard-row-meta"><span>{authorLabel(note.author)}</span><time datetime={note.created_at}>{dateLabel(note.created_at)}</time></span></span></a>{:else}<p class="dashboard-empty">No notes yet.</p>{/each}{/if}</div>
  {/if}
</section>
