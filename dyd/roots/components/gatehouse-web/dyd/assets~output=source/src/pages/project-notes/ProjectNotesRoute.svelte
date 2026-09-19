<script lang="ts">
  import { X } from "@lucide/svelte"
  import { useRuntime } from "../../app/runtime.svelte"
  import Modal from "../../components/Modal.svelte"
  import PageBody from "../../components/PageBody.svelte"
  import PageHeading from "../../components/PageHeading.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import StatusPage from "../../components/StatusPage.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import { fetchProject, type Project } from "../../app/projects"
  import {
    createProjectNote,
    fetchProjectNote,
    fetchProjectNoteRevision,
    fetchProjectNoteRevisions,
    fetchProjectNotes,
    removeProjectNote,
    updateProjectNote,
    type ProjectNote,
  } from "../../app/project-notes"
  import { renderMarkdown } from "../../markdown"
  import type { Route } from "../../route"
  import {
    acceptsProjectNotesResult,
    activateProjectNotesRoute,
    settledProjectNotesListStatus,
  } from "./lifecycle"

  type NotesRoute = Extract<
    Route,
    { kind: "project-notes" | "project-note-new" | "project-note" }
  >
  type Status = "checking" | "ready" | "unavailable"
  type Revision = Pick<
    ProjectNote,
    "title" | "description" | "sensitive" | "author" | "created_at"
  > & { revision: number; body?: string }

  const runtime = useRuntime()
  const { activity, access, auth } = runtime
  let project = $state<Project | null>(null)
  let projectStatus = $state<Status>("checking")
  let notes = $state<ProjectNote[]>([])
  let notesStatus = $state<Status>("checking")
  let active = $state<ProjectNote | null>(null)
  let detailStatus = $state<Status>("ready")
  let creating = $state(false)
  let editing = $state(false)
  let saving = $state(false)
  let deleting = $state(false)
  let title = $state("")
  let description = $state("")
  let body = $state("")
  let sensitive = $state(false)
  let error = $state("")
  let historyModal = $state<HTMLDialogElement | undefined>()
  let history = $state<Revision[]>([])
  let historyLoading = $state(false)
  let historyError = $state("")
  let selectedRevision = $state<Revision | null>(null)
  let revisionLoading = $state(false)
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as NotesRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const listPath = (workspaceID: string, projectID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}/pnt`
  const detailPath = (workspaceID: string, projectID: string, noteID: string) =>
    `${listPath(workspaceID, projectID)}/${encodeURIComponent(noteID)}`
  const projectPath = (workspaceID: string, projectID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const dateLabel = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }
  const authorLabel = (author: ProjectNote["author"]) =>
    author.principal?.name ??
    author.principal?.id ??
    author.agent?.label ??
    author.agent?.id ??
    author.gateway ??
    "Unknown"
  const sortNotes = (loaded: ProjectNote[]) =>
    [...loaded].sort((left, right) => {
      const difference =
        new Date(right.created_at).getTime() -
        new Date(left.created_at).getTime()
      return Number.isFinite(difference) && difference !== 0
        ? difference
        : right.id.localeCompare(left.id)
    })
  const isCurrent = (value: number, workspaceID: string, projectID: string) =>
    acceptsProjectNotesResult(
      generation,
      value,
      currentRoute.workspaceID,
      workspaceID,
      currentRoute.projectID,
      projectID,
    )

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: NotesRoute): () => void {
    const value = ++generation
    const routeState = activateProjectNotesRoute(route.kind)
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    project = null
    projectStatus = "checking"
    notes = []
    notesStatus = routeState.notesStatus
    active = null
    detailStatus = routeState.detailStatus
    creating = routeState.creating
    editing = routeState.editing
    saving = false
    deleting = false
    title = ""
    description = ""
    body = ""
    sensitive = false
    error = ""
    selectedRevision = null
    history = []
    historyError = ""
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    ) {
      void loadRoute(route, value, abortController.signal)
    } else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    ) {
      void runtime.refresh()
    }
    return () => {
      if (value === generation) {
        abortController?.abort()
        unsubscribe?.()
        unsubscribe = undefined
      }
    }
  }

  async function loadRoute(
    route: NotesRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<void> {
    const workspaceID = route.workspaceID
    const projectID = route.projectID
    const selectedWorkspace = access.state.workspaces.find(
      (candidate) => candidate.id === workspaceID,
    )
    if (selectedWorkspace === undefined) {
      if (isCurrent(value, workspaceID, projectID))
        runtime.navigate(
          access.state.workspaces.length === 0
            ? "/app/no-access"
            : `/app/wsp/${encodeURIComponent(access.state.workspaces[0].id)}`,
          true,
        )
      return
    }
    try {
      const response = await fetchProject(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (response.status === 404) {
        runtime.navigate(
          `/app/wsp/${encodeURIComponent(workspaceID)}/prj`,
          true,
        )
        return
      }
      if (!response.ok) throw new Error("project unavailable")
      const loaded = (await response.json()) as Project
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return
      project = loaded
      projectStatus = "ready"
      subscribeToNotes(route, value)
      await Promise.all([
        loadNotes(value, workspaceID, projectID, signal),
        ...(route.kind === "project-note"
          ? [loadDetail(route.noteID, value, workspaceID, projectID, signal)]
          : []),
      ])
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        projectStatus = "unavailable"
    }
  }

  function subscribeToNotes(route: NotesRoute, value: number): void {
    const { workspaceID, projectID } = route
    unsubscribe = activity.subscribe(
      [
        {
          name: "project-notes",
          topic: `${workspaceID}/${projectID}`,
          events: ["project_note.*"],
        },
      ],
      async ({ signal }) => {
        if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return
        const loaded = await loadNotes(
          value,
          workspaceID,
          projectID,
          signal,
          false,
        )
        const detail =
          route.kind !== "project-note" ||
          (await loadDetail(
            route.noteID,
            value,
            workspaceID,
            projectID,
            signal,
          ))
        if (!loaded || !detail || signal.aborted)
          throw new Error("project notes refresh failed")
      },
    )
  }

  async function loadNotes(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, projectID))
      notesStatus = "checking"
    try {
      const response = await fetchProjectNotes(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("notes unavailable")
      const loaded = (await response.json()) as ProjectNote[]
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      notes = sortNotes(loaded)
      notesStatus = settledProjectNotesListStatus(true)
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        notesStatus = settledProjectNotesListStatus(false)
      return false
    }
  }

  async function loadDetail(
    noteID: string,
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
  ): Promise<boolean> {
    detailStatus = "checking"
    try {
      const response = await fetchProjectNote(
        workspaceID,
        projectID,
        noteID,
        signal,
      )
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(listPath(workspaceID, projectID), true)
        return false
      }
      if (!response.ok) throw new Error("note unavailable")
      const loaded = (await response.json()) as ProjectNote
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      active = loaded
      detailStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        detailStatus = "unavailable"
      return false
    }
  }

  function startEdit(): void {
    if (active === null) return
    title = active.title
    description = active.description
    body = active.body ?? ""
    sensitive = active.sensitive
    error = ""
    editing = true
  }

  function cancelEdit(): void {
    if (saving) return
    error = ""
    if (creating)
      runtime.navigate(
        listPath(currentRoute.workspaceID, currentRoute.projectID),
      )
    else editing = false
  }

  async function save(): Promise<void> {
    if (title.trim() === "") {
      error = "Title is required."
      return
    }
    const route = currentRoute
    const value = generation
    const input = { title, description, body, sensitive }
    const note = active
    saving = true
    error = ""
    try {
      const response = creating
        ? await createProjectNote(
            route.workspaceID,
            route.projectID,
            input,
            abortController?.signal,
          )
        : note === null
          ? undefined
          : await updateProjectNote(
              route.workspaceID,
              route.projectID,
              note.id,
              input,
              abortController?.signal,
            )
      if (
        response === undefined ||
        !isCurrent(value, route.workspaceID, route.projectID)
      )
        return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("note could not be saved")
      const saved = (await response.json()) as ProjectNote
      if (!isCurrent(value, route.workspaceID, route.projectID)) return
      notes = sortNotes([
        saved,
        ...notes.filter((candidate) => candidate.id !== saved.id),
      ])
      notesStatus = "ready"
      runtime.navigate(detailPath(route.workspaceID, route.projectID, saved.id))
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID))
        error = "The note could not be saved. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID)) saving = false
    }
  }

  async function remove(): Promise<void> {
    if (
      active === null ||
      deleting ||
      !window.confirm(`Remove ${active.title}?`)
    )
      return
    const route = currentRoute
    const note = active
    const value = generation
    deleting = true
    error = ""
    try {
      const response = await removeProjectNote(
        route.workspaceID,
        route.projectID,
        note.id,
        abortController?.signal,
      )
      if (
        !isCurrent(value, route.workspaceID, route.projectID) ||
        active?.id !== note.id
      )
        return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("note could not be removed")
      notes = notes.filter((candidate) => candidate.id !== note.id)
      runtime.navigate(listPath(route.workspaceID, route.projectID))
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID))
        error = "The note could not be removed. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID)) deleting = false
    }
  }

  async function openHistory(): Promise<void> {
    if (active === null) return
    const route = currentRoute
    const note = active
    const value = generation
    history = []
    historyError = ""
    historyLoading = true
    if (!historyModal?.open) historyModal?.showModal()
    try {
      const response = await fetchProjectNoteRevisions(
        route.workspaceID,
        route.projectID,
        note.id,
        abortController?.signal,
      )
      if (
        !isCurrent(value, route.workspaceID, route.projectID) ||
        active?.id !== note.id
      )
        return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("history unavailable")
      const loaded = (await response.json()) as Revision[]
      if (
        !isCurrent(value, route.workspaceID, route.projectID) ||
        active?.id !== note.id ||
        abortController?.signal.aborted
      )
        return
      history = [...loaded].sort(
        (left, right) => right.revision - left.revision,
      )
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID))
        historyError = "The note history could not be loaded. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID))
        historyLoading = false
    }
  }

  async function selectRevision(revision: number): Promise<void> {
    if (active === null || revision === active.revision) {
      selectedRevision = null
      historyModal?.close()
      return
    }
    const route = currentRoute
    const note = active
    const value = generation
    revisionLoading = true
    historyError = ""
    try {
      const response = await fetchProjectNoteRevision(
        route.workspaceID,
        route.projectID,
        note.id,
        revision,
        abortController?.signal,
      )
      if (
        !isCurrent(value, route.workspaceID, route.projectID) ||
        active?.id !== note.id
      )
        return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("revision unavailable")
      const loaded = (await response.json()) as Revision
      if (
        !isCurrent(value, route.workspaceID, route.projectID) ||
        active?.id !== note.id ||
        abortController?.signal.aborted
      )
        return
      selectedRevision = loaded
      historyModal?.close()
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID))
        historyError = "The note revision could not be loaded. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID))
        revisionLoading = false
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage eyebrow="Gatehouse" busy live="polite">
    {#snippet children()}
      <p>
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </p>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage eyebrow="Gatehouse" title="Connection unavailable" description="Gatehouse could not load your account.">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></div>{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage eyebrow="Gatehouse" title="Sign in required">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => runtime.requireLogin()}>Sign in</button></div>{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage eyebrow="Gatehouse" title="No workspace access" description="Ask an administrator to add you to a workspace group.">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => runtime.navigate("/app/no-access", true)}>Continue</button></div>{/snippet}
  </StatusPage>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><WorkspaceNavigation
        {workspace}
        active="projects"
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <nav aria-label="Breadcrumb">
          <ol>
            <li><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}`}>{workspace.name ?? workspace.id}</RouterLink></li>
            <li><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}>Projects</RouterLink></li>
            <li><RouterLink href={projectPath(workspace.id, currentRoute.projectID)}>{project?.name ?? "New Project"}</RouterLink></li>
            {#if currentRoute.kind === "project-notes"}
              <li aria-current="page">Notes</li>
            {:else}
              <li><RouterLink href={listPath(workspace.id, currentRoute.projectID)}>Notes</RouterLink></li>
              <li aria-current="page">{currentRoute.kind === "project-note-new" ? "New Note" : (active?.title ?? "Note")}</li>
            {/if}
          </ol>
        </nav>
      </SidebarPage.Header>
      <SidebarPage.Body
        ><PageBody fluid>
          {#if projectStatus === "checking"}<p class="muted">
              Loading project...
            </p>
          {:else if projectStatus === "unavailable"}<p class="muted">
              Project unavailable.
            </p>
            <button
              class="primary"
              type="button"
              onclick={() =>
                void loadRoute(
                  currentRoute,
                  generation,
                  abortController!.signal,
                )}>Try again</button
            >
          {:else if editing}<form
              class="stack"
              onsubmit={(event) => {
                event.preventDefault()
                void save()
              }}
            >
              <PageHeading>
                <p class="eyebrow">Project Note</p>
                <h2>{creating ? "New Note" : "Edit Note"}</h2>
              </PageHeading>
              <div class="field">
                <label for="project-note-title">Title</label>
                <input
                  id="project-note-title"
                  autocomplete="off"
                  maxlength="256"
                  required
                  bind:value={title}
                />
              </div>
              <div class="field">
                <label for="project-note-description"
                  >Description (optional)</label
                >
                <textarea
                  id="project-note-description"
                  autocomplete="off"
                  rows="3"
                  maxlength="4096"
                  bind:value={description}></textarea>
              </div>
              <div class="field">
                <label for="project-note-body"
                  >Content (optional)</label
                >
                <textarea
                  id="project-note-body"
                  autocomplete="off"
                  rows="18"
                  maxlength="1048576"
                  bind:value={body}></textarea>
              </div>
              <div class="field">
                <label class="choice"
                  ><input
                    type="checkbox"
                    autocomplete="off"
                    bind:checked={sensitive}
                  /> Sensitive: content is marked sensitive when agents read it.</label
                >
              </div>
              {#if error !== ""}<p class="field-help" role="alert" aria-live="polite">
                  {error}
                </p>{/if}
              <div class="cluster">
                <button
                  type="button"
                  disabled={saving}
                  onclick={cancelEdit}>Cancel</button
                ><button
                  class="primary"
                  type="submit"
                  disabled={saving}>{saving ? "Saving..." : "Save note"}</button
                >
              </div>
            </form>
          {:else if currentRoute.kind === "project-note" && detailStatus === "checking"}<p
              class="muted"
            >
              Loading note...
            </p>
          {:else if currentRoute.kind === "project-note" && detailStatus === "unavailable"}<p
              class="muted"
            >
              Note unavailable.
            </p>
            <button
              class="primary"
              type="button"
              onclick={() =>
                void loadDetail(
                  currentRoute.noteID,
                  generation,
                  currentRoute.workspaceID,
                  currentRoute.projectID,
                  abortController!.signal,
                )}>Try again</button
            >
          {:else if active !== null}{@const displayed =
              selectedRevision ?? active}
            <article class="stack">
              {#snippet noteActions()}{#if selectedRevision === null}<button
                    class="small"
                    type="button"
                    onclick={startEdit}>Edit</button
                  >{:else}<button
                    class="small"
                    type="button"
                    onclick={() => (selectedRevision = null)}
                    >Current revision</button
                  >{/if}<button
                  class="small"
                  type="button"
                  onclick={() => void openHistory()}>History</button
                >{#if selectedRevision === null}<button
                    class="secondary small"
                    type="button"
                    disabled={deleting}
                    onclick={() => void remove()}
                    >{deleting ? "Removing..." : "Remove"}</button
                  >{/if}{/snippet}
              <PageHeading as="header" actions={noteActions}>
                <p class="eyebrow">
                  {selectedRevision === null
                    ? "Project Note"
                    : `Project Note Revision ${selectedRevision.revision}`}
                </p>
                <h2>
                  <span
                    >{displayed.title}{#if displayed.sensitive}<span
                        class="badge">Sensitive</span
                      >{/if}</span
                  >
                </h2>
                {#if displayed.description !== ""}<p>
                    {displayed.description}
                  </p>{/if}<small
                  >By {authorLabel(displayed.author)} on {dateLabel(
                    displayed.created_at,
                  )}</small
                >
              </PageHeading>
              {#if displayed.body !== undefined && displayed.body !== ""}<div
                  class="prose"
                >
                  {@html renderMarkdown(displayed.body)}
                </div>{/if}{#if error !== ""}<p
                  class="field-help"
                  role="alert"
                  aria-live="polite"
                >
                  {error}
                </p>{/if}
            </article>
          {:else}{#snippet collectionActions()}<button
                class="primary small"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${listPath(workspace.id, currentRoute.projectID)}/new`,
                  )}>New note</button
              >{/snippet}
            <PageHeading actions={collectionActions}>
              <h2>Project Notes</h2>
            </PageHeading>
            <div class="list">
              {#if notesStatus === "checking"}<p class="muted">
                  Loading notes...
                </p>{:else if notesStatus === "unavailable"}<p
                  class="muted"
                >
                  Notes could not be loaded.
                </p>
                <button
                class="primary small"
                  type="button"
                  onclick={() =>
                    void loadNotes(
                      generation,
                      currentRoute.workspaceID,
                      currentRoute.projectID,
                      abortController!.signal,
                    )}>Try again</button
                >{:else}{#each notes as note (note.id)}<RouterLink
                    class="list-item surface stack"
                    href={detailPath(
                      workspace.id,
                      currentRoute.projectID,
                      note.id,
                    )}
                    ><span class="stack"
                      ><span
                        >{note.title}{#if note.sensitive}<span
                            class="badge">Sensitive</span
                          >{/if}</span
                      >{#if note.description !== ""}<span
                          class="muted"
                          >{note.description}</span
                        >{/if}<small class="cluster muted"
                        ><time datetime={note.created_at}
                          >{dateLabel(note.created_at)}</time
                        ></small
                      ></span
                    ></RouterLink
                  >{:else}<p class="muted">
                    No notes yet.
                  </p>{/each}{/if}
            </div>{/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
  <Modal
    bind:modal={historyModal}
    labelledBy="note-history-heading"
    onClose={() => {
      historyError = ""
      revisionLoading = false
    }}
  >
    {#snippet header()}<h2 id="note-history-heading">Revision history</h2>
      <button
        class="icon"
        type="button"
        aria-label="Close"
        onclick={() => historyModal?.close()}
        ><X size={18} strokeWidth={2} aria-hidden="true" /></button
      >{/snippet}
    {#if historyLoading}<p class="muted">
        Loading history...
      </p>{:else}{#if historyError !== ""}<p
          class="field-help"
          role="alert"
          aria-live="polite"
        >
          {historyError}
        </p>{/if}
      <div class="list">
        {#each history as revision (revision.revision)}<button
            class="list-item surface up stack"
            data-highlighted={selectedRevision?.revision === revision.revision || undefined}
            type="button"
            disabled={revisionLoading}
            onclick={() => void selectRevision(revision.revision)}
            ><span class="stack"
              ><span
                >Revision {revision.revision}: {revision.title}{#if revision.sensitive}<span
                    class="badge">Sensitive</span
                  >{/if}</span
              >{#if revision.description !== ""}<span
                  class="muted">{revision.description}</span
                >{/if}<small class="cluster muted"
                ><span>{authorLabel(revision.author)}</span><time
                  datetime={revision.created_at}
                  >{dateLabel(revision.created_at)}</time
                ></small
              ></span
            ></button
          >{:else}<p class="muted">No revisions found.</p>{/each}
      </div>
      {#if revisionLoading}<p class="muted">
          Opening revision...
        </p>{/if}{/if}
  </Modal>
{/if}
