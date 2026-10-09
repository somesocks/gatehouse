<script lang="ts">
  import {
    X,
    Modal,
    PageBody,
    PageHeading,
    RouterLink,
    StatusPage,
    renderMarkdown,
  } from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import * as SidebarPage from "../../ui/sidebar-page"

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
  let historyModal = $state<UI.DialogHandle | undefined>()
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
      <UI.Text as="p">
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </UI.Text>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage
    eyebrow="Gatehouse"
    title="Connection unavailable"
    description="Gatehouse could not load your account."
  >
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => void runtime.refresh()}>Try again</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage eyebrow="Gatehouse" title="Sign in required">
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => runtime.requireLogin()}>Sign in</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage
    eyebrow="Gatehouse"
    title="No workspace access"
    description="Ask an administrator to add you to a workspace group."
  >
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => runtime.navigate("/app/no-access", true)}
          >Continue</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "projects")}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb">
          <UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}`}
                >{workspace.name ?? workspace.id}</RouterLink
              ></UI.Breadcrumb.Item
            >
            <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
              ><RouterLink
                href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}
                >Projects</RouterLink
              ></UI.Breadcrumb.Item
            >
            <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
              ><RouterLink
                href={projectPath(workspace.id, currentRoute.projectID)}
                >{project?.name ?? "New Project"}</RouterLink
              ></UI.Breadcrumb.Item
            >
            {#if currentRoute.kind === "project-notes"}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page>Notes</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {:else}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                ><RouterLink
                  href={listPath(workspace.id, currentRoute.projectID)}
                  >Notes</RouterLink
                ></UI.Breadcrumb.Item
              >
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page
                  >{currentRoute.kind === "project-note-new"
                    ? "New Note"
                    : (active?.title ?? "Note")}</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {/if}
          </UI.Breadcrumb.List>
        </UI.Breadcrumb.Root>
      </SidebarPage.Header>
      <SidebarPage.Body
        ><PageBody fluid>
          {#if projectStatus === "checking"}<UI.Text as="p" tone="muted">
              Loading project...
            </UI.Text>
          {:else if projectStatus === "unavailable"}<UI.Text
              as="p"
              tone="muted"
            >
              Project unavailable.
            </UI.Text>
            <UI.Button
              variant="default"
              type="button"
              onclick={() =>
                void loadRoute(
                  currentRoute,
                  generation,
                  abortController!.signal,
                )}>Try again</UI.Button
            >
          {:else if editing}<UI.Form
              onsubmit={(event) => {
                event.preventDefault()
                void save()
              }}
            >
              <PageHeading>
                <UI.Text as="p" size="eyebrow">Project Note</UI.Text>
                <UI.Heading level={2}
                  >{creating ? "New Note" : "Edit Note"}</UI.Heading
                >
              </PageHeading>
              <UI.Field>
                <UI.Label for="project-note-title">Title</UI.Label>
                <UI.Input
                  id="project-note-title"
                  autocomplete="off"
                  maxlength={256}
                  required
                  bind:value={title}
                />
              </UI.Field>
              <UI.Field>
                <UI.Label for="project-note-description"
                  >Description (optional)</UI.Label
                >
                <UI.Textarea
                  id="project-note-description"
                  autocomplete="off"
                  rows={3}
                  maxlength={4096}
                  bind:value={description}
                ></UI.Textarea>
              </UI.Field>
              <UI.Field>
                <UI.Label for="project-note-body">Content (optional)</UI.Label>
                <UI.Textarea
                  id="project-note-body"
                  autocomplete="off"
                  rows={18}
                  maxlength={1048576}
                  bind:value={body}
                ></UI.Textarea>
              </UI.Field>
              <UI.Field>
                <UI.Label layout="choice"
                  ><UI.Checkbox bind:checked={sensitive} /> Sensitive: content is
                  marked sensitive when agents read it.</UI.Label
                >
              </UI.Field>
              {#if error !== ""}<UI.Text
                  as="p"
                  size="small"
                  role="alert"
                  aria-live="polite"
                >
                  {error}
                </UI.Text>{/if}
              <UI.Row>
                <UI.Button
                  variant="outline"
                  type="button"
                  disabled={saving}
                  onclick={cancelEdit}>Cancel</UI.Button
                ><UI.Button variant="default" type="submit" disabled={saving}
                  >{saving ? "Saving..." : "Save note"}</UI.Button
                >
              </UI.Row>
            </UI.Form>
          {:else if currentRoute.kind === "project-note" && detailStatus === "checking"}<UI.Text
              as="p"
              tone="muted"
            >
              Loading note...
            </UI.Text>
          {:else if currentRoute.kind === "project-note" && detailStatus === "unavailable"}<UI.Text
              as="p"
              tone="muted"
            >
              Note unavailable.
            </UI.Text>
            <UI.Button
              variant="default"
              type="button"
              onclick={() =>
                void loadDetail(
                  currentRoute.noteID,
                  generation,
                  currentRoute.workspaceID,
                  currentRoute.projectID,
                  abortController!.signal,
                )}>Try again</UI.Button
            >
          {:else if active !== null}{@const displayed =
              selectedRevision ?? active}
            {#snippet noteActions()}{#if selectedRevision === null}<UI.Button
                  variant="outline"
                  size="sm"
                  type="button"
                  onclick={startEdit}>Edit</UI.Button
                >{:else}<UI.Button
                  variant="outline"
                  size="sm"
                  type="button"
                  onclick={() => (selectedRevision = null)}
                  >Current revision</UI.Button
                >{/if}<UI.Button
                variant="outline"
                size="sm"
                type="button"
                onclick={() => void openHistory()}>History</UI.Button
              >{#if selectedRevision === null}<UI.Button
                  variant="outline"
                  size="sm"
                  type="button"
                  disabled={deleting}
                  onclick={() => void remove()}
                  >{deleting ? "Removing..." : "Remove"}</UI.Button
                >{/if}{/snippet}
            <UI.Stack as="article">
              <PageHeading as="header" actions={noteActions}>
                <UI.Text as="p" size="eyebrow">
                  {selectedRevision === null
                    ? "Project Note"
                    : `Project Note Revision ${selectedRevision.revision}`}
                </UI.Text>
                <UI.Heading level={2}>
                  <UI.Text
                    >{displayed.title}{#if displayed.sensitive}<UI.Badge
                        >Sensitive</UI.Badge
                      >{/if}</UI.Text
                  >
                </UI.Heading>
                {#if displayed.description !== ""}<UI.Text as="p">
                    {displayed.description}
                  </UI.Text>{/if}<UI.Text as="small" size="small"
                  >By {authorLabel(displayed.author)} on {dateLabel(
                    displayed.created_at,
                  )}</UI.Text
                >
              </PageHeading>
              {#if displayed.body !== undefined && displayed.body !== ""}<UI.RichText
                  html={renderMarkdown(displayed.body)}
                ></UI.RichText>{/if}{#if error !== ""}<UI.Text
                  as="p"
                  size="small"
                  role="alert"
                  aria-live="polite"
                >
                  {error}
                </UI.Text>{/if}
            </UI.Stack>
          {:else}{#snippet collectionActions()}<UI.Button
                variant="default"
                size="sm"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${listPath(workspace.id, currentRoute.projectID)}/new`,
                  )}>New note</UI.Button
              >{/snippet}
            <PageHeading actions={collectionActions}>
              <UI.Heading level={2}>Project Notes</UI.Heading>
            </PageHeading>
            <UI.List>
              {#if notesStatus === "checking"}<UI.Text as="p" tone="muted">
                  Loading notes...
                </UI.Text>{:else if notesStatus === "unavailable"}<UI.Text
                  as="p"
                  tone="muted"
                >
                  Notes could not be loaded.
                </UI.Text>
                <UI.Button
                  variant="default"
                  size="sm"
                  type="button"
                  onclick={() =>
                    void loadNotes(
                      generation,
                      currentRoute.workspaceID,
                      currentRoute.projectID,
                      abortController!.signal,
                    )}>Try again</UI.Button
                >{:else}{#each notes as note (note.id)}<RouterLink
                    variant="item"
                    href={detailPath(
                      workspace.id,
                      currentRoute.projectID,
                      note.id,
                    )}
                    ><UI.Stack as="span"
                      ><UI.Text
                        >{note.title}{#if note.sensitive}<UI.Badge
                            >Sensitive</UI.Badge
                          >{/if}</UI.Text
                      >{#if note.description !== ""}<UI.Text tone="muted"
                          >{note.description}</UI.Text
                        >{/if}<UI.Row tone="muted" as="small"
                        ><UI.Text as="time" datetime={note.created_at}
                          >{dateLabel(note.created_at)}</UI.Text
                        ></UI.Row
                      ></UI.Stack
                    ></RouterLink
                  >{:else}<UI.Text as="p" tone="muted">
                    No notes yet.
                  </UI.Text>{/each}{/if}
            </UI.List>{/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
  <Modal
    title="Revision history"
    bind:modal={historyModal}
    labelledBy="note-history-heading"
    onClose={() => {
      historyError = ""
      revisionLoading = false
    }}
  >
    {#snippet header()}<UI.Heading level={2} id="note-history-heading"
        >Revision history</UI.Heading
      >
      <UI.Button
        variant="ghost"
        size="icon"
        type="button"
        aria-label="Close"
        onclick={() => historyModal?.close()}
        ><X size={18} strokeWidth={2} aria-hidden="true" /></UI.Button
      >{/snippet}
    {#if historyLoading}<UI.Text as="p" tone="muted">
        Loading history...
      </UI.Text>{:else}{#if historyError !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {historyError}
        </UI.Text>{/if}
      <UI.List>
        {#each history as revision (revision.revision)}<UI.Item
            interactive
            selected={Boolean(
              selectedRevision?.revision === revision.revision || undefined,
            )}
            type="button"
            disabled={revisionLoading}
            onclick={() => void selectRevision(revision.revision)}
            ><UI.Stack as="span"
              ><UI.Text
                >Revision {revision.revision}: {revision.title}{#if revision.sensitive}<UI.Badge
                    >Sensitive</UI.Badge
                  >{/if}</UI.Text
              >{#if revision.description !== ""}<UI.Text tone="muted"
                  >{revision.description}</UI.Text
                >{/if}<UI.Row tone="muted" as="small"
                ><UI.Text>{authorLabel(revision.author)}</UI.Text><UI.Text
                  as="time"
                  datetime={revision.created_at}
                  >{dateLabel(revision.created_at)}</UI.Text
                ></UI.Row
              ></UI.Stack
            ></UI.Item
          >{:else}<UI.Text as="p" tone="muted">No revisions found.</UI.Text
          >{/each}
      </UI.List>
      {#if revisionLoading}<UI.Text as="p" tone="muted">
          Opening revision...
        </UI.Text>{/if}{/if}
  </Modal>
{/if}
