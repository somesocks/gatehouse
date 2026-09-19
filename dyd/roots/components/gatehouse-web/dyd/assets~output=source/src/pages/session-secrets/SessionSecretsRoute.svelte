<script lang="ts">
  import { fetchChatSession } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import SessionNavigation from "../../components/SessionNavigation.svelte"
  import StatusPage from "../../components/StatusPage.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import type { Route } from "../../route"
  import SessionSecretsPage from "./SessionSecretsPage.svelte"

  type SecretsRoute = Extract<
    Route,
    { kind: "session-secrets" | "session-secret-new" | "session-secret" }
  >
  type Session = {
    id: string
    created_at: string
    name?: string
    project?: { id: string; name?: string }
  }
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let session = $state<Session | null>(null)
  let sessionStatus = $state<Status>("checking")
  let secretBreadcrumb = $state<string | null>(null)
  let generation = 0
  let abortController: AbortController | null = null
  let routeSignal = $state<AbortSignal | null>(null)
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as SecretsRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`
  function updateSessionName(name: string): void {
    if (session !== null) session = { ...session, name }
  }
  const sessionsPath = (workspaceID: string) =>
    `${workspacePath(workspaceID)}/ses`
  const sessionPath = (workspaceID: string, sessionID: string) =>
    `${sessionsPath(workspaceID)}/${encodeURIComponent(sessionID)}`
  const sessionSecretsPath = (workspaceID: string, sessionID: string) =>
    `${sessionPath(workspaceID, sessionID)}/secrets`
  const projectPath = (workspaceID: string, projectID: string) =>
    `${workspacePath(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const isCurrent = (
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
  ) =>
    value === generation &&
    !signal.aborted &&
    currentRoute.workspaceID === workspaceID &&
    currentRoute.sessionID === sessionID

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: SecretsRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    routeSignal = abortController.signal
    unsubscribe?.()
    unsubscribe = undefined
    session = null
    sessionStatus = "checking"
    secretBreadcrumb = null
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    )
      void loadRoute(route, value, abortController.signal)
    else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    )
      void runtime.refresh()
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
    const { workspaceID, sessionID } = route
    if (
      !access.state.workspaces.some((candidate) => candidate.id === workspaceID)
    ) {
      if (isCurrent(value, workspaceID, sessionID, signal))
        runtime.navigate(
          access.state.workspaces.length === 0
            ? "/app/no-access"
            : workspacePath(access.state.workspaces[0].id),
          true,
        )
      return
    }
    if (!(await loadSession(value, workspaceID, sessionID, signal))) return
    if (!isCurrent(value, workspaceID, sessionID, signal)) return
    subscribe(route, value, signal)
  }

  async function loadSession(
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, sessionID, signal))
      sessionStatus = "checking"
    try {
      const response = await fetchChatSession(workspaceID, sessionID, signal)
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(sessionsPath(workspaceID), true)
        return false
      }
      if (!response.ok) throw new Error("session unavailable")
      const loaded = (await response.json()) as Session
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      session = loaded
      sessionStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        sessionStatus = "unavailable"
      return false
    }
  }

  function subscribe(
    route: SecretsRoute,
    value: number,
    routeSignal: AbortSignal,
  ): void {
    const { workspaceID, sessionID } = route
    unsubscribe = activity.subscribe(
      [
        {
          name: "session-secrets",
          topic: `${workspaceID}/${sessionID}`,
          events: ["session.*"],
        },
      ],
      async ({ signal }) => {
        if (
          !isCurrent(value, workspaceID, sessionID, routeSignal) ||
          signal.aborted
        )
          return
        if (
          !(await loadSession(
            value,
            workspaceID,
            sessionID,
            routeSignal,
            false,
          )) ||
          signal.aborted ||
          !isCurrent(value, workspaceID, sessionID, routeSignal)
        )
          throw new Error("session secrets refresh failed")
      },
    )
    void activity.poll()
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
      ><WorkspaceNavigation {workspace} active="chats" /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <nav aria-label="Breadcrumb">
          <ol>
            <li><RouterLink href={workspacePath(workspace.id)}>{workspace.name ?? workspace.id}</RouterLink></li>
            {#if session !== null}
              {#if session.project !== undefined}
                <li><RouterLink href={projectPath(workspace.id, session.project.id)}>{session.project.name ?? "New Project"}</RouterLink></li>
              {/if}
              <li><RouterLink href={sessionPath(workspace.id, session.id)}>{session.name ?? "New Chat"}</RouterLink></li>
              {#if currentRoute.kind === "session-secrets"}
                <li aria-current="page">Secrets</li>
              {:else}
                <li><RouterLink href={sessionSecretsPath(workspace.id, session.id)}>Secrets</RouterLink></li>
                {#if secretBreadcrumb !== null}<li aria-current="page">{secretBreadcrumb}</li>{/if}
              {/if}
            {/if}
          </ol>
        </nav>
        {#if session !== null}<SessionNavigation
            workspaceID={workspace.id}
            sessionID={session.id}
            active="secrets"
            name={session.name}
            onRenamed={updateSessionName}
          />{/if}</SidebarPage.Header
      >
      <SidebarPage.Body
        >{#if sessionStatus === "checking"}<p
            class="muted"
            aria-busy="true"
            aria-live="polite"
          >
            Loading chat...
          </p>
        {:else if sessionStatus === "unavailable"}<p class="muted">
            This chat could not be loaded.
          </p>
          <button
            class="primary"
            type="button"
            onclick={() => {
              const signal = abortController?.signal
              if (signal !== undefined)
                void loadRoute(currentRoute, generation, signal)
            }}>Try again</button
          >
        {:else if session !== null && routeSignal !== null}<SessionSecretsPage
            workspaceID={workspace.id}
            sessionID={session.id}
            route={currentRoute}
            signal={routeSignal}
            {activity}
            onAuthenticationLost={() => runtime.requireLogin()}
            onNavigate={(path, replace) => runtime.navigate(path, replace)}
            onBreadcrumbChange={(title) => (secretBreadcrumb = title)}
          />
        {/if}</SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
