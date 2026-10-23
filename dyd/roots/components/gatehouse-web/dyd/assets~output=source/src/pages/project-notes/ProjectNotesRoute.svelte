<script lang="ts">
  import { untrack } from "svelte"
  import { createLoadState } from "../../utils/load-state.svelte"
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
  import { userAuthorLabel } from "../../app/user-labels"
  import { createCollectionPage } from "../../utils/collection-page.svelte"
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
    {
      kind:
        | "project-notes"
        | "project-note-new"
        | "project-note"
        | "project-note-edit"
        | "project-note-revision"
    }
  >
  type Status = "checking" | "ready" | "unavailable"
  type Revision = Pick<
    ProjectNote,
    "title" | "description" | "sensitive" | "author" | "created_at"
  > & { revision: number; body?: string }

  const runtime = useRuntime()
  const projectLoad = createLoadState()
  const notesLoad = createLoadState()
  const detailLoad = createLoadState()
  const page = createCollectionPage()
  const historyPage = createCollectionPage()
  const reloadHistory = async () => {
    await openHistory(false)
    return historyError === ""
  }
  const reloadNotes = () =>
    loadNotes(
      generation,
      currentRoute.workspaceID,
      currentRoute.projectID,
      abortController!.signal,
    )
  const { activity, access, auth } = runtime
  let project = $state<Project | null>(null)
  const projectStatus = $derived(projectLoad.status)
  let notes = $state<ProjectNote[]>([])
  const notesStatus = $derived(notesLoad.status)
  let active = $state<ProjectNote | null>(null)
  const detailStatus = $derived(detailLoad.status)
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
  let historyRequest: AbortController | undefined
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
  const authorLabel = userAuthorLabel
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

  const activation = $derived(
    JSON.stringify([
      currentRoute,
      auth.state.status,
      auth.state.claims?.principal.ref.id,
      access.state.workspaceStatus,
      workspace !== null,
    ]),
  )
  $effect(() => {
    activation
    return untrack(() => activate(currentRoute))
  })

  function activate(route: NotesRoute): () => void {
    page.reset()
    projectLoad.reset()
    notesLoad.reset()
    detailLoad.reset()
    const value = ++generation
    const routeState = activateProjectNotesRoute(route.kind)
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    project = null
    notes = []
    active = null
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
        projectLoad.stop()
        notesLoad.stop()
        detailLoad.stop()
        historyRequest?.abort()
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
    const request = projectLoad.begin(signal)
    signal = request.signal
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
      if (
        !request.commit(() => {
          project = loaded
        })
      )
        return
      subscribeToNotes(route, value)
      await Promise.all([
        ...(route.kind === "project-notes"
          ? [loadNotes(value, workspaceID, projectID, signal)]
          : []),
        ...("noteID" in route
          ? [loadDetail(route.noteID, value, workspaceID, projectID, signal)]
          : []),
      ])
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        request.fail(new Error("Project could not be loaded."))
    } finally {
      request.finish()
    }
  }

  function subscribeToNotes(route: NotesRoute, value: number): void {
    unsubscribe?.()
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
        const loaded =
          route.kind !== "project-notes" ||
          (await loadNotes(value, workspaceID, projectID, signal, false))
        const detail =
          !("noteID" in route) ||
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
    const request = notesLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const requested = page.key()
      const response = await fetchProjectNotes(
        workspaceID,
        projectID,
        signal,
        page.params(),
      )
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
      if (requested !== page.key()) return false
      return request.commit(() => {
        page.accept(response)
        notes = loaded
      })
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        request.fail(new Error("Notes could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  async function loadDetail(
    noteID: string,
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
  ): Promise<boolean> {
    const request = detailLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
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
      if (currentRoute.kind === "project-note-revision") {
        const response = await fetchProjectNoteRevision(
          workspaceID,
          projectID,
          noteID,
          currentRoute.revision,
          signal,
        )
        if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
          return false
        if (!response.ok) throw new Error("Revision could not be loaded.")
        const revision = (await response.json()) as Revision
        if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
          return false
        selectedRevision = revision
      }
      return request.commit(() => {
        active = loaded
        if (currentRoute.kind === "project-note-edit" && !editing)
          populateEdit()
      })
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        request.fail(new Error("Note could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  function populateEdit(): void {
    if (active === null) return
    title = active.title
    description = active.description
    body = active.body ?? ""
    sensitive = active.sensitive
    error = ""
    editing = true
  }

  async function save(): Promise<void> {
    if (saving || deleting) return
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

  async function openHistory(reset = true): Promise<void> {
    if (active === null) return
    const parentSignal = abortController?.signal
    if (!parentSignal) return
    historyRequest?.abort()
    const request = new AbortController()
    historyRequest = request
    const signal = AbortSignal.any([parentSignal, request.signal])
    if (reset) historyPage.reset()
    const route = currentRoute
    const note = active
    const value = generation
    if (reset) history = []
    historyError = ""
    historyLoading = true
    if (!historyModal?.open) historyModal?.showModal()
    try {
      const response = await fetchProjectNoteRevisions(
        route.workspaceID,
        route.projectID,
        note.id,
        signal,
        historyPage.params(),
      )
      if (
        !isCurrent(value, route.workspaceID, route.projectID) ||
        active?.id !== note.id ||
        signal.aborted
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
        signal.aborted
      )
        return
      historyPage.accept(response)
      history = [...loaded].sort(
        (left, right) => right.revision - left.revision,
      )
    } catch {
      if (
        isCurrent(value, route.workspaceID, route.projectID) &&
        !signal.aborted
      )
        historyError = "The note history could not be loaded. Try again."
    } finally {
      if (
        isCurrent(value, route.workspaceID, route.projectID) &&
        !signal.aborted
      )
        historyLoading = false
    }
  }

  function revisionPath(revision: number): string {
    if (active === null)
      return listPath(currentRoute.workspaceID, currentRoute.projectID)
    const path = detailPath(currentRoute.workspaceID, currentRoute.projectID, active.id)
    return revision === active.revision ? path : `${path}/revisions/${revision}`
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
      ><UI.RouterLink
        variant="default"
        href={runtime.loginHref} replace>Sign in</UI.RouterLink
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
      ><UI.RouterLink
        variant="default"
        href="/app/no-access" replace
        >Continue</UI.RouterLink
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
                >{workspace.name ?? "Workspace"}</RouterLink
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
          {#if projectLoad.reloading || notesLoad.reloading || detailLoad.reloading}<UI.Text
              role="status">Refreshing notes…</UI.Text
            >{/if}
          {#if projectLoad.state.error || notesLoad.state.error || detailLoad.state.error}<UI.Notice
              variant="destructive"
              role="alert"
              >{projectLoad.state.error ||
                notesLoad.state.error ||
                detailLoad.state.error}<UI.Button
                variant="outline"
                onclick={() =>
                  void loadRoute(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Retry</UI.Button
              ></UI.Notice
            >{/if}
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
          {:else if editing}<UI.AdminHeader
              title={creating ? "New note" : `Edit ${active?.title ?? "note"}`}
              description="Update the summary, content, and sensitivity."
            /><UI.Form
              surface
              onsubmit={(event) => {
                event.preventDefault()
                void save()
              }}
            >
              <UI.FormSection
                title="Summary"
                description="A title and short description identify this note."
                ><UI.Field>
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
              </UI.FormSection><UI.FormSection
                title="Content"
                description="Write the note using Markdown."
                ><UI.Field>
                  <UI.Label for="project-note-body">Content (optional)</UI.Label
                  >
                  <UI.Textarea
                    id="project-note-body"
                    autocomplete="off"
                    rows={18}
                    maxlength={1048576}
                    bind:value={body}
                  ></UI.Textarea>
                </UI.Field>
              </UI.FormSection><UI.FormSection
                title="Sensitivity"
                description="Control how agents handle the note content."
                ><UI.Field>
                  <UI.Label layout="choice"
                    ><UI.Checkbox bind:checked={sensitive} /> Sensitive: content
                    is marked sensitive when agents read it.</UI.Label
                  >
                </UI.Field>
              </UI.FormSection>{#if error !== ""}<UI.Text
                  as="p"
                  size="small"
                  role="alert"
                  aria-live="polite"
                >
                  {error}
                </UI.Text>{/if}
              <UI.FormActions>
                <UI.RouterLink
                  variant="outline"
                  disabled={saving}
                  href={creating || active === null
                    ? listPath(currentRoute.workspaceID, currentRoute.projectID)
                    : detailPath(currentRoute.workspaceID, currentRoute.projectID, active.id)}>Cancel</UI.RouterLink
                ><UI.Button variant="default" type="submit" disabled={saving}
                  >{saving ? "Saving..." : "Save note"}</UI.Button
                >
              </UI.FormActions>
            </UI.Form>
          {:else if "noteID" in currentRoute && detailStatus === "checking"}<UI.Text
              as="p"
              tone="muted"
            >
              Loading note...
            </UI.Text>
          {:else if "noteID" in currentRoute && detailStatus === "unavailable"}<UI.Text
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
          {:else if active !== null}{@const noteID = active.id}{@const displayed =
              selectedRevision ?? active}
            {#snippet noteActions()}{#if selectedRevision === null}<UI.RouterLink
                  variant="outline"
                  size="sm"
                  href={`${detailPath(currentRoute.workspaceID, currentRoute.projectID, noteID)}/edit`}>Edit</UI.RouterLink
                >{:else}<UI.RouterLink
                  variant="outline"
                  size="sm"
                  href={detailPath(currentRoute.workspaceID, currentRoute.projectID, noteID)}>Current revision</UI.RouterLink
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
              <UI.AdminHeader
                kind={selectedRevision === null
                  ? "Project note"
                  : `Historical revision ${selectedRevision.revision}`}
                title={displayed.title}
                description={displayed.description}
                actions={noteActions}
              />
              <UI.DetailSection title="Note information">
                <UI.Property label="Author"
                  >{authorLabel(displayed.author)}</UI.Property
                >
                <UI.Property label="Date"
                  >{dateLabel(displayed.created_at)}</UI.Property
                >
                <UI.Property label="Revision">{displayed.revision}</UI.Property>
                <UI.Property label="Sensitivity"
                  >{displayed.sensitive ? "Sensitive" : "Standard"}</UI.Property
                >
              </UI.DetailSection>
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
          {:else}{#snippet collectionActions()}<UI.RouterLink
                variant="default"
                size="sm"
                href={`${listPath(workspace.id, currentRoute.projectID)}/new`}>New note</UI.RouterLink
              >{/snippet}
            <UI.AdminHeader
              title="Notes"
              description="Project documents and their revision history."
              actions={collectionActions}
            />
            <UI.SearchToolbar
              label="Search notes"
              placeholder="Title or description"
              bind:value={page.state.search}
              loading={page.state.busy || notesLoad.state.pending}
              onsearch={() => void page.search(reloadNotes)}
            />
            <UI.RecordTable
              headers={["Title", "Sensitivity", "Author", "Date"]}
            >
              {#if notesStatus !== "ready"}<UI.TableEmpty
                  columns={4}
                  message={notesStatus === "unavailable"
                    ? "Notes could not be loaded."
                    : notesStatus === "checking"
                      ? "Loading notes…"
                      : "Notes could not be loaded."}
                />
              {:else}{#each notes as note (note.id)}<UI.RecordRow>
                    <UI.RecordCell primary
                      ><RouterLink
                        href={detailPath(
                          workspace.id,
                          currentRoute.projectID,
                          note.id,
                        )}>{note.title}</RouterLink
                      ><UI.Text size="small" tone="muted"
                        >{note.description}</UI.Text
                      ></UI.RecordCell
                    >
                    <UI.RecordCell
                      >{note.sensitive
                        ? "Sensitive"
                        : "Standard"}</UI.RecordCell
                    >
                    <UI.RecordCell>{authorLabel(note.author)}</UI.RecordCell>
                    <UI.RecordCell>{dateLabel(note.created_at)}</UI.RecordCell>
                  </UI.RecordRow>{:else}<UI.TableEmpty
                    columns={4}
                    message="No notes yet."
                  />{/each}{/if}
            </UI.RecordTable>
            <UI.PaginationFooter
              noun="notes"
              count={notes.length}
              previous={page.state.history.length > 0}
              next={!!page.state.next}
              loading={page.state.busy || notesLoad.state.pending}
              onprevious={() => void page.previous(reloadNotes)}
              onnext={() => void page.next(reloadNotes)}
            />
            {#if notesStatus === "unavailable"}<UI.Button
                variant="outline"
                onclick={() =>
                  void loadNotes(
                    generation,
                    currentRoute.workspaceID,
                    currentRoute.projectID,
                    abortController!.signal,
                  )}>Try again</UI.Button
              >{/if}
          {/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
  <Modal
    title="Revision history"
    bind:modal={historyModal}
    labelledBy="note-history-heading"
    onClose={() => {
      historyRequest?.abort()
      historyLoading = false
      historyPage.reset()
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
      <UI.SearchToolbar
        label="Search revisions"
        placeholder="Title or revision"
        bind:value={historyPage.state.search}
        loading={historyLoading || historyPage.state.busy}
        onsearch={() => void historyPage.search(reloadHistory)}
      />
      <UI.RecordTable headers={["Revision", "Title", "Author", "Date"]}>
        {#each history as revision (revision.revision)}<UI.RecordRow>
            <UI.RecordCell primary
              ><UI.RouterLink
                variant="link"
                href={revisionPath(revision.revision)}
                onNavigate={() => historyModal?.close()}
                >Revision {revision.revision}</UI.RouterLink
              >{#if revision.revision === active?.revision}<UI.Badge
                  >Current</UI.Badge
                >{/if}</UI.RecordCell
            >
            <UI.RecordCell
              >{revision.title}{#if revision.sensitive}<UI.Badge
                  >Sensitive</UI.Badge
                >{/if}</UI.RecordCell
            >
            <UI.RecordCell>{authorLabel(revision.author)}</UI.RecordCell>
            <UI.RecordCell>{dateLabel(revision.created_at)}</UI.RecordCell>
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={4}
            message="No matching revisions."
          />{/each}
      </UI.RecordTable>
      <UI.PaginationFooter
        noun="revisions"
        count={history.length}
        previous={historyPage.state.history.length > 0}
        next={!!historyPage.state.next}
        loading={historyLoading || historyPage.state.busy}
        onprevious={() => void historyPage.previous(reloadHistory)}
        onnext={() => void historyPage.next(reloadHistory)}
      />
      {#if revisionLoading}<UI.Text as="p" tone="muted">
          Opening revision...
        </UI.Text>{/if}{/if}
  </Modal>
{/if}
