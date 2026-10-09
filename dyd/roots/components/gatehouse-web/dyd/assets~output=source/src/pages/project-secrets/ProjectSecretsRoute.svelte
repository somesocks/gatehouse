<script lang="ts">
  import { PageBody, PageHeading, RouterLink, StatusPage } from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import * as UI from "../../ui"
  import { fetchProject, type Project } from "../../app/projects"
  import {
    createProjectSecret,
    fetchProjectSecret,
    fetchProjectSecrets,
    removeProjectSecret,
    updateProjectSecret,
    type ProjectSecret,
  } from "../../app/project-secrets"
  import { useRuntime } from "../../app/runtime.svelte"
  import * as SidebarPage from "../../ui/sidebar-page"

  import type { Route } from "../../route"

  type SecretsRoute = Extract<
    Route,
    { kind: "project-secrets" | "project-secret-new" | "project-secret" }
  >
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { activity, access, auth } = runtime
  let project = $state<Project | null>(null)
  let projectStatus = $state<Status>("checking")
  let secrets = $state<ProjectSecret[]>([])
  let secretsStatus = $state<Status>("checking")
  let active = $state<ProjectSecret | null>(null)
  let detailStatus = $state<Status>("ready")
  let creating = $state(false)
  let editing = $state(false)
  let saving = $state(false)
  let deleting = $state(false)
  let description = $state("")
  let secretValue = $state("")
  let error = $state("")
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as SecretsRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const listPath = (workspaceID: string, projectID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}/secrets`
  const detailPath = (
    workspaceID: string,
    projectID: string,
    secretID: string,
  ) => `${listPath(workspaceID, projectID)}/${encodeURIComponent(secretID)}`
  const projectPath = (workspaceID: string, projectID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const dateLabel = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }
  const authorLabel = (author: ProjectSecret["author"]) =>
    author.name ?? author.id
  const sortSecrets = (loaded: ProjectSecret[]) =>
    [...loaded].sort((left, right) => {
      const difference =
        new Date(right.updated_at).getTime() -
        new Date(left.updated_at).getTime()
      return Number.isFinite(difference) && difference !== 0
        ? difference
        : right.id.localeCompare(left.id)
    })
  const isCurrent = (value: number, workspaceID: string, projectID: string) =>
    value === generation &&
    currentRoute.workspaceID === workspaceID &&
    currentRoute.projectID === projectID &&
    !abortController?.signal.aborted

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: SecretsRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    project = null
    projectStatus = "checking"
    secrets = []
    secretsStatus = "checking"
    active = null
    detailStatus = route.kind === "project-secret" ? "checking" : "ready"
    creating = route.kind === "project-secret-new"
    editing = creating
    saving = false
    deleting = false
    description = ""
    secretValue = ""
    error = ""
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
    route: SecretsRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<void> {
    const { workspaceID, projectID } = route
    if (
      !access.state.workspaces.some((candidate) => candidate.id === workspaceID)
    ) {
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
      subscribe(route, value)
      await Promise.all([
        loadSecrets(value, workspaceID, projectID, signal),
        ...(route.kind === "project-secret"
          ? [loadDetail(route.secretID, value, workspaceID, projectID, signal)]
          : []),
      ])
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        projectStatus = "unavailable"
    }
  }

  function subscribe(route: SecretsRoute, value: number): void {
    const { workspaceID, projectID } = route
    unsubscribe = activity.subscribe(
      [
        {
          name: "project-secrets",
          topic: `${workspaceID}/${projectID}`,
          events: ["project_secret.*"],
        },
      ],
      async ({ signal }) => {
        if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return
        const listed = await loadSecrets(
          value,
          workspaceID,
          projectID,
          signal,
          false,
        )
        const detailed =
          route.kind !== "project-secret" ||
          (await loadDetail(
            route.secretID,
            value,
            workspaceID,
            projectID,
            signal,
          ))
        if (
          !listed ||
          !detailed ||
          signal.aborted ||
          !isCurrent(value, workspaceID, projectID)
        )
          throw new Error("project secrets refresh failed")
      },
    )
    void activity.poll()
  }

  async function loadSecrets(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, projectID))
      secretsStatus = "checking"
    try {
      const response = await fetchProjectSecrets(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("secrets unavailable")
      const loaded = (await response.json()) as ProjectSecret[]
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      secrets = sortSecrets(loaded)
      secretsStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        secretsStatus = "unavailable"
      return false
    }
  }

  async function loadDetail(
    secretID: string,
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
  ): Promise<boolean> {
    if (isCurrent(value, workspaceID, projectID)) detailStatus = "checking"
    try {
      const response = await fetchProjectSecret(
        workspaceID,
        projectID,
        secretID,
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
      if (!response.ok) throw new Error("secret unavailable")
      const loaded = (await response.json()) as ProjectSecret
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
    description = active.description
    secretValue = ""
    error = ""
    editing = true
  }

  function cancelEdit(): void {
    if (saving) return
    error = ""
    secretValue = ""
    if (creating)
      runtime.navigate(
        listPath(currentRoute.workspaceID, currentRoute.projectID),
      )
    else editing = false
  }

  async function save(): Promise<void> {
    if (description.trim() === "") {
      error = "Description is required."
      return
    }
    if (creating && secretValue === "") {
      error = "Value is required."
      return
    }
    const route = currentRoute
    const value = generation
    const secret = active
    const input: { description: string; value?: string } = { description }
    if (creating || secretValue !== "") input.value = secretValue
    saving = true
    error = ""
    try {
      const response = creating
        ? await createProjectSecret(
            route.workspaceID,
            route.projectID,
            { description: input.description, value: input.value ?? "" },
            abortController?.signal,
          )
        : secret === null
          ? undefined
          : await updateProjectSecret(
              route.workspaceID,
              route.projectID,
              secret.id,
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
      if (!response.ok) throw new Error("secret could not be saved")
      const saved = (await response.json()) as ProjectSecret
      if (
        !isCurrent(value, route.workspaceID, route.projectID) ||
        abortController?.signal.aborted
      )
        return
      secretValue = ""
      secrets = sortSecrets([
        saved,
        ...secrets.filter((candidate) => candidate.id !== saved.id),
      ])
      secretsStatus = "ready"
      runtime.navigate(detailPath(route.workspaceID, route.projectID, saved.id))
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID))
        error = "The secret could not be saved. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID)) saving = false
    }
  }

  async function remove(): Promise<void> {
    if (
      active === null ||
      deleting ||
      !window.confirm(`Remove ${active.description}?`)
    )
      return
    const route = currentRoute
    const secret = active
    const value = generation
    deleting = true
    error = ""
    try {
      const response = await removeProjectSecret(
        route.workspaceID,
        route.projectID,
        secret.id,
        abortController?.signal,
      )
      if (
        !isCurrent(value, route.workspaceID, route.projectID) ||
        active?.id !== secret.id
      )
        return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("secret could not be removed")
      secrets = secrets.filter((candidate) => candidate.id !== secret.id)
      runtime.navigate(listPath(route.workspaceID, route.projectID))
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID))
        error = "The secret could not be removed. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID)) deleting = false
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
            {#if currentRoute.kind === "project-secrets"}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page>Secrets</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {:else}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                ><RouterLink
                  href={listPath(workspace.id, currentRoute.projectID)}
                  >Secrets</RouterLink
                ></UI.Breadcrumb.Item
              >
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page
                  >{currentRoute.kind === "project-secret-new"
                    ? "New Secret"
                    : (active?.description ?? "Secret")}</UI.Breadcrumb.Page
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
                <UI.Text as="p" size="eyebrow">Project Secret</UI.Text>
                <UI.Heading level={2}
                  >{creating ? "New Secret" : "Edit Secret"}</UI.Heading
                >
              </PageHeading>
              <UI.Field>
                <UI.Label for="project-secret-description">Description</UI.Label
                >
                <UI.Textarea
                  id="project-secret-description"
                  autocomplete="off"
                  rows={3}
                  maxlength={4096}
                  required
                  bind:value={description}
                ></UI.Textarea>
              </UI.Field>
              <UI.Field>
                <UI.Label for="project-secret-value"
                  >{creating ? "Value" : "New value (optional)"}</UI.Label
                >
                <UI.Textarea
                  id="project-secret-value"
                  autocomplete="new-password"
                  rows={5}
                  maxlength={1048576}
                  required={creating}
                  bind:value={secretValue}
                ></UI.Textarea>
                {#if !creating}<UI.Text as="p" size="small">
                    Leave blank to keep the current value.
                  </UI.Text>{/if}
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
                  >{saving ? "Saving..." : "Save secret"}</UI.Button
                >
              </UI.Row>
            </UI.Form>
          {:else if currentRoute.kind === "project-secret" && detailStatus === "checking"}<UI.Text
              as="p"
              tone="muted"
            >
              Loading secret...
            </UI.Text>
          {:else if currentRoute.kind === "project-secret" && detailStatus === "unavailable"}<UI.Text
              as="p"
              tone="muted"
            >
              Secret unavailable.
            </UI.Text>
            <UI.Button
              variant="default"
              type="button"
              onclick={() =>
                void loadDetail(
                  currentRoute.secretID,
                  generation,
                  currentRoute.workspaceID,
                  currentRoute.projectID,
                  abortController!.signal,
                )}>Try again</UI.Button
            >
          {:else if active !== null}{#snippet secretActions()}<UI.Button
                variant="outline"
                size="sm"
                type="button"
                onclick={startEdit}>Edit</UI.Button
              ><UI.Button
                variant="outline"
                size="sm"
                type="button"
                disabled={deleting}
                onclick={() => void remove()}
                >{deleting ? "Removing..." : "Remove"}</UI.Button
              >{/snippet}
            <UI.Stack as="article">
              <PageHeading as="header" actions={secretActions}>
                <UI.Text as="p" size="eyebrow">Project Secret</UI.Text>
                <UI.Heading level={2}>{active.description}</UI.Heading>
                <UI.Text as="small" size="small"
                  >By {authorLabel(active.author)} on {dateLabel(
                    active.created_at,
                  )}{#if active.updated_at !== active.created_at}
                    / Updated {dateLabel(active.updated_at)}{/if}</UI.Text
                >
              </PageHeading>
              {#if error !== ""}<UI.Text
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
                  )}>New secret</UI.Button
              >{/snippet}
            <PageHeading actions={collectionActions}>
              <UI.Heading level={2}>Project Secrets</UI.Heading>
            </PageHeading>
            <UI.List>
              {#if secretsStatus === "checking"}<UI.Text as="p" tone="muted">
                  Loading secrets...
                </UI.Text>{:else if secretsStatus === "unavailable"}<UI.Text
                  as="p"
                  tone="muted"
                >
                  Secrets could not be loaded.
                </UI.Text>
                <UI.Button
                  variant="default"
                  size="sm"
                  type="button"
                  onclick={() =>
                    void loadSecrets(
                      generation,
                      currentRoute.workspaceID,
                      currentRoute.projectID,
                      abortController!.signal,
                    )}>Try again</UI.Button
                >{:else}{#each secrets as secret (secret.id)}<RouterLink
                    variant="item"
                    href={detailPath(
                      workspace.id,
                      currentRoute.projectID,
                      secret.id,
                    )}
                    ><UI.Stack as="span"
                      ><UI.Text>{secret.description}</UI.Text><UI.Row
                        tone="muted"
                        as="small"
                        ><UI.Text>{authorLabel(secret.author)}</UI.Text><UI.Text
                          as="time"
                          datetime={secret.updated_at}
                          >Updated {dateLabel(secret.updated_at)}</UI.Text
                        ></UI.Row
                      ></UI.Stack
                    ></RouterLink
                  >{:else}<UI.Text as="p" tone="muted">
                    No secrets yet.
                  </UI.Text>{/each}{/if}
            </UI.List>{/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
