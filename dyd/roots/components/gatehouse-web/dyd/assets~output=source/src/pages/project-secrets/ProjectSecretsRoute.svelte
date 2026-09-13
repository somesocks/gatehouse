<script lang="ts">
  import { Menu } from "@lucide/svelte"
  import { signOut } from "../../app/auth"
  import { fetchProject, type Project } from "../../app/projects"
  import { createProjectSecret, fetchProjectSecret, fetchProjectSecrets, removeProjectSecret, updateProjectSecret, type ProjectSecret } from "../../app/project-secrets"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import WorkspaceFrame from "../../components/WorkspaceFrame.svelte"
  import type { Route } from "../../route"

  type SecretsRoute = Extract<Route, { kind: "project-secrets" | "project-secret-new" | "project-secret" }>
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { activity, access, auth } = runtime
  let mobileMenuOpen = $state(false)
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
  const workspace = $derived(access.state.workspaces.find((candidate) => candidate.id === currentRoute.workspaceID) ?? null)
  const listPath = (workspaceID: string, projectID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}/secrets`
  const detailPath = (workspaceID: string, projectID: string, secretID: string) => `${listPath(workspaceID, projectID)}/${encodeURIComponent(secretID)}`
  const projectPath = (workspaceID: string, projectID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const dateLabel = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }
  const authorLabel = (author: ProjectSecret["author"]) => author.name ?? author.id
  const sortSecrets = (loaded: ProjectSecret[]) => [...loaded].sort((left, right) => {
    const difference = new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
    return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
  })
  const isCurrent = (value: number, workspaceID: string, projectID: string) => value === generation && currentRoute.workspaceID === workspaceID && currentRoute.projectID === projectID && !abortController?.signal.aborted

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
    mobileMenuOpen = false
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
    if (auth.state.status === "authenticated" && access.state.workspaceStatus === "ready") {
      void loadRoute(route, value, abortController.signal)
    } else if (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking") {
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

  async function loadRoute(route: SecretsRoute, value: number, signal: AbortSignal): Promise<void> {
    const { workspaceID, projectID } = route
    if (!access.state.workspaces.some((candidate) => candidate.id === workspaceID)) {
      if (isCurrent(value, workspaceID, projectID)) runtime.navigate(access.state.workspaces.length === 0 ? "/app/no-access" : `/app/wsp/${encodeURIComponent(access.state.workspaces[0].id)}`, true)
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
        runtime.navigate(`/app/wsp/${encodeURIComponent(workspaceID)}/prj`, true)
        return
      }
      if (!response.ok) throw new Error("project unavailable")
      const loaded = await response.json() as Project
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return
      project = loaded
      projectStatus = "ready"
      subscribe(route, value)
      await Promise.all([loadSecrets(value, workspaceID, projectID, signal), ...(route.kind === "project-secret" ? [loadDetail(route.secretID, value, workspaceID, projectID, signal)] : [])])
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted) projectStatus = "unavailable"
    }
  }

  function subscribe(route: SecretsRoute, value: number): void {
    const { workspaceID, projectID } = route
    unsubscribe = activity.subscribe([{ name: "project-secrets", topic: `${workspaceID}/${projectID}`, events: ["project_secret.*"] }], async ({ signal }) => {
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return
      const listed = await loadSecrets(value, workspaceID, projectID, signal, false)
      const detailed = route.kind !== "project-secret" || await loadDetail(route.secretID, value, workspaceID, projectID, signal)
      if (!listed || !detailed || signal.aborted || !isCurrent(value, workspaceID, projectID)) throw new Error("project secrets refresh failed")
    })
    void activity.poll()
  }

  async function loadSecrets(value: number, workspaceID: string, projectID: string, signal: AbortSignal, showLoading = true): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, projectID)) secretsStatus = "checking"
    try {
      const response = await fetchProjectSecrets(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("secrets unavailable")
      const loaded = await response.json() as ProjectSecret[]
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return false
      secrets = sortSecrets(loaded)
      secretsStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted) secretsStatus = "unavailable"
      return false
    }
  }

  async function loadDetail(secretID: string, value: number, workspaceID: string, projectID: string, signal: AbortSignal): Promise<boolean> {
    if (isCurrent(value, workspaceID, projectID)) detailStatus = "checking"
    try {
      const response = await fetchProjectSecret(workspaceID, projectID, secretID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(listPath(workspaceID, projectID), true)
        return false
      }
      if (!response.ok) throw new Error("secret unavailable")
      const loaded = await response.json() as ProjectSecret
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return false
      active = loaded
      detailStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted) detailStatus = "unavailable"
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
    if (creating) runtime.navigate(listPath(currentRoute.workspaceID, currentRoute.projectID))
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
      const response = creating ? await createProjectSecret(route.workspaceID, route.projectID, { description: input.description, value: input.value ?? "" }, abortController?.signal) : secret === null ? undefined : await updateProjectSecret(route.workspaceID, route.projectID, secret.id, input, abortController?.signal)
      if (response === undefined || !isCurrent(value, route.workspaceID, route.projectID)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("secret could not be saved")
      const saved = await response.json() as ProjectSecret
      if (!isCurrent(value, route.workspaceID, route.projectID) || abortController?.signal.aborted) return
      secretValue = ""
      secrets = sortSecrets([saved, ...secrets.filter((candidate) => candidate.id !== saved.id)])
      secretsStatus = "ready"
      runtime.navigate(detailPath(route.workspaceID, route.projectID, saved.id))
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID)) error = "The secret could not be saved. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID)) saving = false
    }
  }

  async function remove(): Promise<void> {
    if (active === null || deleting || !window.confirm(`Remove ${active.description}?`)) return
    const route = currentRoute
    const secret = active
    const value = generation
    deleting = true
    error = ""
    try {
      const response = await removeProjectSecret(route.workspaceID, route.projectID, secret.id, abortController?.signal)
      if (!isCurrent(value, route.workspaceID, route.projectID) || active?.id !== secret.id) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("secret could not be removed")
      secrets = secrets.filter((candidate) => candidate.id !== secret.id)
      runtime.navigate(listPath(route.workspaceID, route.projectID))
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID)) error = "The secret could not be removed. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID)) deleting = false
    }
  }

  async function logout(): Promise<void> {
    try {
      await signOut()
    } finally {
      runtime.requireLogin()
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="auth-shell" aria-busy="true" aria-live="polite"><section class="status-card"><p class="eyebrow">Gatehouse</p><div class="loading-mark" aria-hidden="true"></div><p>{auth.state.status === "checking" ? "Checking your session." : "Loading your workspaces."}</p></section></main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">Connection unavailable</h1><p class="subtitle is-6">Gatehouse could not load your account.</p><button class="button is-primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></section></main>
{:else if auth.state.status !== "authenticated"}
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">Sign in required</h1><button class="button is-primary" type="button" onclick={() => runtime.requireLogin()}>Sign in</button></section></main>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">No workspace access</h1><p class="subtitle is-6">Ask an administrator to add you to a workspace group.</p><button class="button is-primary" type="button" onclick={() => runtime.navigate("/app/no-access", true)}>Continue</button></section></main>
{:else}
  <WorkspaceFrame {mobileMenuOpen} brandTheme onMenuClose={() => mobileMenuOpen = false}>
    {#snippet sidebar()}
      <RouterLink class="brand" href="/app/">Gatehouse</RouterLink>
      <div class="workspace-switcher"><label for="workspace">Workspace</label><div class="select is-fullwidth"><select id="workspace" value={workspace.id} onchange={(event) => runtime.navigate(`/app/wsp/${encodeURIComponent(event.currentTarget.value)}`)}>{#each access.state.workspaces as candidate (candidate.id)}<option value={candidate.id}>{candidate.name ?? candidate.id}</option>{/each}</select></div></div>
      <nav class="sidebar-nav" aria-label="Workspace navigation"><section class="sidebar-section"><ul><li><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}/ses`}>Chats</RouterLink></li><li><RouterLink class="active" href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}>Projects</RouterLink></li><li><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}/grp`}>Groups</RouterLink></li></ul></section></nav>
      {#if access.state.systemAccess === "available"}<div class="sidebar-system-link"><RouterLink href="/app/system" target="_blank" rel="noopener">System</RouterLink></div>{/if}
      <div class="sidebar-footer"><span>{auth.state.claims?.principal.name ?? "User"}</span><button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button></div>
    {/snippet}
    {#snippet header()}
      <button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}><Menu size={20} strokeWidth={2} aria-hidden="true" /></button>
      <h1 class="workspace-breadcrumb"><RouterLink class="workspace-breadcrumb-segment" href={`/app/wsp/${encodeURIComponent(workspace.id)}`}><span>{workspace.name ?? workspace.id}</span></RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}>Projects</RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><RouterLink class="workspace-breadcrumb-segment" href={projectPath(workspace.id, currentRoute.projectID)}><span>{project?.name ?? "New Project"}</span></RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>{#if currentRoute.kind === "project-secrets"}<span>Secrets</span>{:else}<RouterLink href={listPath(workspace.id, currentRoute.projectID)}>Secrets</RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><span class="workspace-breadcrumb-segment">{currentRoute.kind === "project-secret-new" ? "New Secret" : active?.description ?? "Secret"}</span>{/if}</h1>
    {/snippet}
    <section class="project-note-page">
      {#if projectStatus === "checking"}<p class="dashboard-empty">Loading project...</p>
      {:else if projectStatus === "unavailable"}<p class="dashboard-empty">Project unavailable.</p><button class="button is-primary" type="button" onclick={() => void loadRoute(currentRoute, generation, abortController!.signal)}>Try again</button>
      {:else if editing}<form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void save() }}><div class="project-note-page-heading"><div><p class="eyebrow">Project Secret</p><h2>{creating ? "New Secret" : "Edit Secret"}</h2></div></div><div class="field"><label class="label" for="project-secret-description">Description</label><div class="control"><textarea class="textarea" id="project-secret-description" autocomplete="off" rows="3" maxlength="4096" required bind:value={description}></textarea></div></div><div class="field"><label class="label" for="project-secret-value">{creating ? "Value" : "New value (optional)"}</label><div class="control"><textarea class="textarea" id="project-secret-value" autocomplete="new-password" rows="5" maxlength="1048576" required={creating} bind:value={secretValue}></textarea></div>{#if !creating}<p class="help">Leave blank to keep the current value.</p>{/if}</div>{#if error !== ""}<p class="help is-danger" aria-live="polite">{error}</p>{/if}<div class="project-note-actions"><button class="button" type="button" disabled={saving} onclick={cancelEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={saving}>{saving ? "Saving..." : "Save secret"}</button></div></form>
      {:else if currentRoute.kind === "project-secret" && detailStatus === "checking"}<p class="dashboard-empty">Loading secret...</p>
      {:else if currentRoute.kind === "project-secret" && detailStatus === "unavailable"}<p class="dashboard-empty">Secret unavailable.</p><button class="button is-primary" type="button" onclick={() => void loadDetail(currentRoute.secretID, generation, currentRoute.workspaceID, currentRoute.projectID, abortController!.signal)}>Try again</button>
      {:else if active !== null}<article class="project-note-view"><header class="project-note-page-heading"><div><p class="eyebrow">Project Secret</p><h2>{active.description}</h2><small>By {authorLabel(active.author)} on {dateLabel(active.created_at)}{#if active.updated_at !== active.created_at} / Updated {dateLabel(active.updated_at)}{/if}</small></div><div class="project-note-actions"><button class="button is-small" type="button" onclick={startEdit}>Edit</button><button class="button is-small is-danger is-light" type="button" disabled={deleting} onclick={() => void remove()}>{deleting ? "Removing..." : "Remove"}</button></div></header>{#if error !== ""}<p class="help is-danger" aria-live="polite">{error}</p>{/if}</article>
      {:else}<div class="collection-heading project-collection-heading"><h2>Project Secrets</h2><RouterLink class="button is-primary is-small" href={`${listPath(workspace.id, currentRoute.projectID)}/new`}>New secret</RouterLink></div><div class="collection-list">{#if secretsStatus === "checking"}<p class="dashboard-empty">Loading secrets...</p>{:else if secretsStatus === "unavailable"}<p class="dashboard-empty">Secrets could not be loaded.</p><button class="button is-primary is-small" type="button" onclick={() => void loadSecrets(generation, currentRoute.workspaceID, currentRoute.projectID, abortController!.signal)}>Try again</button>{:else}{#each secrets as secret (secret.id)}<RouterLink class="dashboard-row project-note-row" href={detailPath(workspace.id, currentRoute.projectID, secret.id)}><span class="dashboard-row-content"><span class="project-note-title">{secret.description}</span><span class="dashboard-row-meta"><span>{authorLabel(secret.author)}</span><time datetime={secret.updated_at}>Updated {dateLabel(secret.updated_at)}</time></span></span></RouterLink>{:else}<p class="dashboard-empty">No secrets yet.</p>{/each}{/if}</div>{/if}
    </section>
  </WorkspaceFrame>
{/if}
