<script lang="ts">
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../app/activity"
  import { projectNotesAPIPath, type ProjectNote } from "../../app/project-notes"
  import { renderMarkdown } from "../../markdown"
  import type { Route } from "../../route"
  import { createProjectNotesController } from "./project-notes-controller.svelte"

  type NoteRevision = { revision: number; title: string; description: string; sensitive: boolean; author: ProjectNote["author"]; created_at: string; body?: string }
  let { workspaceID, projectID, route, activity, previewNotes, previewStatus, selectedRevision, onAuthenticationLost, onNavigate, onBreadcrumbChange, onPreviewChanged, onResetHistory, onOpenHistory, onShowCurrentRevision }: { workspaceID: string; projectID: string; route: Route; activity: ActivityClient; previewNotes: ProjectNote[]; previewStatus: "checking" | "ready" | "unavailable"; selectedRevision: NoteRevision | null; onAuthenticationLost: () => void; onNavigate: (path: string, replace?: boolean) => void; onBreadcrumbChange: (title: string | null) => void; onPreviewChanged: () => Promise<boolean>; onResetHistory: () => void; onOpenHistory: (notesPath: string, noteID: string, revision: number) => void; onShowCurrentRevision: () => void } = $props()
  const controller = untrack(() => createProjectNotesController({ activity, onAuthenticationLost, onNavigate, onPreviewChanged, onResetHistory }))

  $effect(() => {
    const currentRoute = route
    if (currentRoute.kind === "project-notes" || currentRoute.kind === "project-note-new" || currentRoute.kind === "project-note") return untrack(() => controller.start(workspaceID, projectID, currentRoute))
  })
  $effect(() => { onBreadcrumbChange(controller.state.creating ? "New Note" : controller.state.active?.title ?? null); return () => onBreadcrumbChange(null) })

  const notePath = (id: string) => `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}/pnt/${encodeURIComponent(id)}`
  function authorLabel(author: ProjectNote["author"]) { if (author.principal !== undefined) return author.principal.name ?? author.principal.id; if (author.agent !== undefined) return author.agent.label ?? author.agent.id; return author.gateway ?? "Unknown" }
  function dateLabel(value: string) { const date = new Date(value); if (Number.isNaN(date.getTime())) return value; const number = (part: number) => part.toString().padStart(2, "0"); return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}` }
</script>

<section class="project-note-page">
  {#if controller.state.editing}
    <form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void controller.save() }}>
      <div class="project-note-page-heading"><div><p class="eyebrow">Project Note</p><h2>{controller.state.creating ? "New Note" : "Edit Note"}</h2></div></div>
      <div class="field"><label class="label" for="project-note-title">Title</label><div class="control"><input class="input" id="project-note-title" autocomplete="off" maxlength="256" required bind:value={controller.state.title} /></div></div>
      <div class="field"><label class="label" for="project-note-description">Description (optional)</label><div class="control"><textarea class="textarea" id="project-note-description" autocomplete="off" rows="3" maxlength="4096" bind:value={controller.state.description}></textarea></div></div>
      <div class="field"><label class="label" for="project-note-body">Content (optional)</label><div class="control"><textarea class="textarea project-note-body-input" id="project-note-body" autocomplete="off" rows="18" maxlength="1048576" bind:value={controller.state.body}></textarea></div></div>
      <div class="field"><label class="checkbox"><input type="checkbox" autocomplete="off" bind:checked={controller.state.sensitive} /> Sensitive: content is marked sensitive when agents read it.</label></div>
      {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
      <div class="project-note-actions"><button class="button" type="button" disabled={controller.state.saving} onclick={() => controller.cancelEdit()}>Cancel</button><button class="button is-primary" type="submit" disabled={controller.state.saving}>{controller.state.saving ? "Saving..." : "Save note"}</button></div>
    </form>
  {:else if controller.state.active !== null}
    {@const note = controller.state.active}
    {@const displayed = selectedRevision ?? note}
    <article class="project-note-view">
      <header class="project-note-page-heading"><div><p class="eyebrow">{selectedRevision === null ? "Project Note" : `Project Note Revision ${selectedRevision.revision}`}</p><h2><span class="project-note-title">{displayed.title}{#if displayed.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span></h2>{#if displayed.description !== ""}<p>{displayed.description}</p>{/if}<small>By {authorLabel(displayed.author)} on {dateLabel(displayed.created_at)}</small></div><div class="project-note-actions">{#if selectedRevision === null}<button class="button is-small" type="button" onclick={() => controller.startEdit()}>Edit</button>{:else}<button class="button is-small" type="button" onclick={onShowCurrentRevision}>Current revision</button>{/if}<button class="button is-small" type="button" onclick={() => onOpenHistory(projectNotesAPIPath(workspaceID, projectID), note.id, note.revision)}>History</button>{#if selectedRevision === null}<button class="button is-small is-danger is-light" type="button" disabled={controller.state.deleting} onclick={() => void controller.remove()}>{controller.state.deleting ? "Removing..." : "Remove"}</button>{/if}</div></header>
      {#if displayed.body !== undefined && displayed.body !== ""}<div class="markdown-content project-note-markdown">{@html renderMarkdown(displayed.body)}</div>{/if}
      {#if controller.state.error !== ""}<p class="help is-danger" aria-live="polite">{controller.state.error}</p>{/if}
    </article>
  {:else if controller.state.loading}
    <p class="dashboard-empty">Loading note...</p>
  {:else}
    <div class="collection-heading"><h2>Project Notes</h2><button class="button is-primary is-small" type="button" onclick={() => controller.startCreate()}>New note</button></div>
    <div class="collection-list">{#if previewStatus === "checking"}<p class="dashboard-empty">Loading notes...</p>{:else if previewStatus === "unavailable"}<p class="dashboard-empty">Notes could not be loaded.</p>{:else}{#each previewNotes as note (note.id)}<a class="dashboard-row project-note-row" href={notePath(note.id)} onclick={(event) => { event.preventDefault(); onNavigate(notePath(note.id)) }}><span class="dashboard-row-content"><span class="project-note-title">{note.title}{#if note.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if note.description !== ""}<span class="project-note-description">{note.description}</span>{/if}<span class="dashboard-row-meta"><time datetime={note.created_at}>{dateLabel(note.created_at)}</time></span></span></a>{:else}<p class="dashboard-empty">No notes yet.</p>{/each}{/if}</div>
  {/if}
</section>
