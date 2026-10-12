<script lang="ts">
  import { onDestroy, untrack } from "svelte"
  import * as UI from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import { useRuntime } from "../../app/runtime.svelte"
  import {
    createProject,
    fetchProject,
    updateProject,
    type Project,
  } from "../../app/projects"
  import type { Route } from "../../route"

  const runtime = useRuntime()
  const route = $derived(
    runtime.state.route as Extract<
      Route,
      { kind: "project-new" | "project-edit" }
    >,
  )
  const workspace = $derived(
    runtime.access.state.workspaces.find(
      (item) => item.id === route.workspaceID,
    ),
  )
  const listPath = $derived(
    `/app/wsp/${encodeURIComponent(route.workspaceID)}/prj`,
  )
  const returnPath = $derived(
    route.kind === "project-edit"
      ? `${listPath}/${encodeURIComponent(route.projectID)}`
      : listPath,
  )
  let project = $state<Project | null>(null)
  let name = $state("")
  let description = $state("")
  let loading = $state(false)
  let loaded = $state(false)
  let saving = $state(false)
  let error = $state("")
  let controller: AbortController
  let contextKey = ""
  onDestroy(() => controller?.abort())

  $effect(() => {
    const selected = route
    const ready =
      runtime.auth.state.status === "authenticated" &&
      runtime.access.state.workspaceStatus === "ready" &&
      !!workspace
    untrack(() => {
      if (!ready) return
      const key = `${selected.kind}/${selected.workspaceID}/${"projectID" in selected ? selected.projectID : ""}`
      if (contextKey === key && loaded) return
      controller?.abort()
      contextKey = key
      controller = new AbortController()
      project = null
      name = ""
      description = ""
      error = ""
      saving = false
      loaded = selected.kind === "project-new"
      loading = selected.kind === "project-edit"
      if (selected.kind === "project-edit")
        void load(selected, controller.signal)
    })
  })

  async function load(
    selected: Extract<Route, { kind: "project-edit" }>,
    signal: AbortSignal,
  ) {
    loading = true
    error = ""
    try {
      const response = await fetchProject(
        selected.workspaceID,
        selected.projectID,
        signal,
      )
      if (signal.aborted) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok)
        throw new Error(
          response.status === 404
            ? "Project was not found."
            : "Project could not be loaded.",
        )
      const result = (await response.json()) as Project
      if (signal.aborted) return
      project = result
      name = result.name ?? ""
      description = result.description ?? ""
      loaded = true
    } catch (cause) {
      if (!signal.aborted)
        error =
          cause instanceof Error
            ? cause.message
            : "Project could not be loaded."
    } finally {
      if (!signal.aborted) loading = false
    }
  }

  async function save() {
    if (saving || !loaded) return
    const selected = route
    const signal = controller.signal
    saving = true
    error = ""
    try {
      const input = { name: name.trim(), description: description.trim() }
      const response =
        selected.kind === "project-new"
          ? await createProject(selected.workspaceID, signal, input)
          : await updateProject(
              selected.workspaceID,
              selected.projectID,
              input,
              signal,
            )
      if (signal.aborted) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("Project could not be saved.")
      if (selected.kind === "project-new") {
        const result = (await response.json()) as Project
        if (!signal.aborted)
          runtime.navigate(`${listPath}/${encodeURIComponent(result.id)}`)
      } else runtime.navigate(returnPath)
    } catch (cause) {
      if (!signal.aborted)
        error =
          cause instanceof Error ? cause.message : "Project could not be saved."
    } finally {
      if (!signal.aborted) saving = false
    }
  }
</script>

{#if runtime.auth.state.status === "checking" || runtime.access.state.workspaceStatus === "checking"}
  <UI.StatusPage
    title="Loading workspace"
    description="Preparing the project form."
    busy
  />
{:else if runtime.auth.state.status === "unavailable" || runtime.access.state.workspaceStatus === "unavailable"}
  <UI.StatusPage title="Connection unavailable"
    ><UI.Button onclick={() => void runtime.refresh()}>Try again</UI.Button
    ></UI.StatusPage
  >
{:else if runtime.auth.state.status !== "authenticated"}
  <UI.StatusPage title="Sign in required"
    ><UI.Button onclick={() => runtime.requireLogin()}>Sign in</UI.Button
    ></UI.StatusPage
  >
{:else if workspace}
  <UI.AppFrame
    navigation={workspaceNavigation(runtime, workspace, "projects")}
    breadcrumbs={[
      {
        label: workspace.name ?? "Workspace",
        href: `/app/wsp/${encodeURIComponent(workspace.id)}`,
      },
      { label: "Projects", href: listPath },
      ...(route.kind === "project-edit"
        ? [{ label: project?.name ?? "Project", href: returnPath }]
        : []),
      { label: route.kind === "project-new" ? "New project" : "Edit" },
    ]}
  >
    <UI.AdminContent mode="form">
      <UI.AdminHeader
        title={route.kind === "project-new"
          ? "New project"
          : `Edit ${project?.name ?? "project"}`}
        description="Organize shared work with a name and description."
      />
      {#if loading}<UI.Text role="status">Loading project…</UI.Text>
      {:else if loaded}<UI.Form
          surface
          onsubmit={(event) => {
            event.preventDefault()
            void save()
          }}
        >
          <UI.FormSection title="Project information"
            ><UI.Field
              ><UI.Label for="project-name">Name</UI.Label><UI.Input
                id="project-name"
                maxlength={256}
                bind:value={name}
              /></UI.Field
            ></UI.FormSection
          >
          <UI.FormSection
            title="Description"
            description="Provide context for people and agents working in this project."
            ><UI.Field
              ><UI.Label for="project-description">Description</UI.Label
              ><UI.Textarea
                id="project-description"
                rows={5}
                maxlength={4096}
                bind:value={description}
              /></UI.Field
            ></UI.FormSection
          >
          {#if error}<UI.Notice variant="destructive" role="alert"
              >{error}</UI.Notice
            >{/if}
          <UI.FormActions
            ><UI.RouterLink variant="outline" href={returnPath}
              >Cancel</UI.RouterLink
            ><UI.Button type="submit" disabled={saving}
              >{saving
                ? "Saving…"
                : route.kind === "project-new"
                  ? "Create project"
                  : "Save changes"}</UI.Button
            ></UI.FormActions
          >
        </UI.Form>
      {:else}<UI.Notice variant="destructive" role="alert">{error}</UI.Notice
        ><UI.Button
          variant="outline"
          onclick={() =>
            route.kind === "project-edit" &&
            void load(route, controller.signal)}>Try again</UI.Button
        >{/if}
    </UI.AdminContent>
  </UI.AppFrame>
{:else}<UI.StatusPage
    title="Workspace unavailable"
    description="Choose a workspace you can access."
    ><UI.RouterLink href="/app/">Choose workspace</UI.RouterLink></UI.StatusPage
  >{/if}
