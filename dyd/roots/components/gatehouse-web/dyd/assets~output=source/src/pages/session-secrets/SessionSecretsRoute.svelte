<script lang="ts">
  import { untrack } from "svelte"
  import { createLoadState } from "../../utils/load-state.svelte"
  import { RouterLink, StatusPage } from "../../ui"
  import {
    workspaceNavigation,
    sessionViews,
    renameSession,
  } from "../../app/navigation"
  import * as UI from "../../ui"
  import SessionViewNavigation from "../../session-participants/SessionViewNavigation.svelte"
  import { fetchChatSession } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"

  import * as SidebarPage from "../../ui/sidebar-page"

  import { routePath, type Route } from "../../route"
  import { sessionToolDismissPath } from "../session-chat/session-tools"
  import { createSessionSecretsController } from "./session-secrets-controller.svelte"
  import type { SessionSecret } from "../../app/session-secrets"
  import SessionSecretsPage from "./SessionSecretsPage.svelte"

  type SecretsRoute = Extract<
    Route,
    {
      kind:
        | "session-secrets"
        | "session-secret-new"
        | "session-secret"
        | "session-secret-edit"
    }
  >
  type Session = {
    id: string
    created_at: string
    name?: string
    project?: { id: string; name?: string }
  }
  type Status = "checking" | "ready" | "unavailable"

  let { embedded = false, route, suspended = false, dialog }: { embedded?: boolean; route?: Route; suspended?: boolean; dialog?: UI.ResourceDialogControls } = $props()
  const runtime = useRuntime()
  const resource = createLoadState()
  const { access, activity, auth } = runtime
  let session = $state<Session | null>(null)
  const sessionStatus = $derived(resource.status)
  const sessionReady = $derived(sessionStatus === "ready" && session !== null)
  const controller = createSessionSecretsController({
    activity: runtime.activity,
    onAuthenticationLost: () => runtime.requireLogin(),
    onNavigate: (path, replace) => runtime.navigate(path, replace),
  })
  let removal = $state<SessionSecret | null>(null)
  let secretBreadcrumb = $derived(controller.state.creating ? "New Secret" : controller.state.active?.description ?? null)
  let generation = 0
  let sessionOwner = ""
  let abortController: AbortController | null = null
  let routeSignal = $state<AbortSignal | null>(null)
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived((route ?? runtime.state.route) as SecretsRoute)
  const editor = $derived(currentRoute.kind === "session-secret-new" || currentRoute.kind === "session-secret-edit")
  const collection = $derived(currentRoute.kind === "session-secrets")
  const dialogTitle = $derived(
    currentRoute.kind === "session-secret-new" ? "New secret"
      : currentRoute.kind === "session-secret-edit" ? "Edit secret"
      : collection ? "Search secrets" : controller.state.active?.description ?? "Secret",
  )
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
    if (suspended) return
    return untrack(() => activate(currentRoute))
  })

  $effect(() => {
    const selected = currentRoute
    const signal = routeSignal
    if (suspended || !sessionReady || !signal) return
    return untrack(() => controller.start(selected.workspaceID, selected.sessionID, selected, signal))
  })

  function requestRemove() {
    if (!controller.state.active || controller.state.deleting) return
    controller.state.error = ""
    removal = { ...controller.state.active }
  }

  function activate(route: SecretsRoute): () => void {
    const value = ++generation
    const owner = JSON.stringify([route.workspaceID, route.sessionID, auth.state.claims?.principal.ref.id])
    if (owner !== sessionOwner) {
      sessionOwner = owner
      resource.reset()
      session = null
    }
    abortController?.abort()
    abortController = new AbortController()
    routeSignal = abortController.signal
    unsubscribe?.()
    unsubscribe = undefined
    removal = null
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
        resource.stop()
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
    const request = resource.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
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
      return request.commit(() => {
        session = loaded
      })
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        request.fail(new Error("Chat could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  function subscribe(
    route: SecretsRoute,
    value: number,
    routeSignal: AbortSignal,
  ): void {
    unsubscribe?.()
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
            AbortSignal.any([routeSignal, signal]),
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

{#snippet searchControls()}
  <UI.SearchToolbar integrated label="Search secrets" placeholder="Search by description…"
    bind:value={controller.page.state.search} loading={controller.page.state.busy || controller.state.status === "checking"}
    onsearch={() => void controller.page.search(controller.reload)} />
{/snippet}

{#snippet viewActions()}
  {#if editor}
    <UI.RouterLink variant="outline" disabled={controller.state.saving} href={sessionToolDismissPath(currentRoute)}>Cancel</UI.RouterLink>
    <UI.Button type="submit" form="session-secret-form" disabled={controller.state.saving || !controller.state.editing || sessionStatus !== "ready"}>{controller.state.saving ? "Saving..." : "Save secret"}</UI.Button>
  {:else if collection}
    <UI.RouterLink variant="default" href={`${sessionSecretsPath(currentRoute.workspaceID, currentRoute.sessionID)}/new`}>New secret</UI.RouterLink>
  {:else if controller.state.active}
    <UI.Button variant="outline" disabled={controller.state.deleting} onclick={requestRemove}>Remove</UI.Button>
    <UI.RouterLink variant={embedded ? "default" : "outline"} href={`${sessionSecretsPath(currentRoute.workspaceID, currentRoute.sessionID)}/${encodeURIComponent(controller.state.active.id)}/edit`}>Edit</UI.RouterLink>
  {/if}
{/snippet}

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
  <UI.ResourcePage {embedded} dialog={dialog ? {
    ...dialog,
    title: dialogTitle,
    viewKey: routePath(currentRoute),
    pageKey: collection ? JSON.stringify([controller.page.state.query, controller.page.state.cursor]) : undefined,
    compact: collection,
    busy: controller.state.saving || controller.state.deleting,
    dismissible: removal === null,
    includeClose: !editor,
    search: collection ? searchControls : undefined,
    footer: viewActions,
  } : undefined}>
    {#snippet sidebar()}<UI.Navigation
        {...workspaceNavigation(runtime, workspace, "chats")}
      />{/snippet}
    {#snippet header()}
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb">
          <UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={workspacePath(workspace.id)}
                >{workspace.name ?? "Workspace"}</RouterLink
              ></UI.Breadcrumb.Item
            >
            {#if session !== null}
              {#if session.project !== undefined}
                <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                  ><RouterLink
                    href={projectPath(workspace.id, session.project.id)}
                    >{session.project.name ?? "New Project"}</RouterLink
                  ></UI.Breadcrumb.Item
                >
              {/if}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                ><RouterLink href={sessionPath(workspace.id, session.id)}
                  >{session.name ?? "New Chat"}</RouterLink
                ></UI.Breadcrumb.Item
              >
              {#if currentRoute.kind === "session-secrets"}
                <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                  aria-current="page"
                  ><UI.Breadcrumb.Page>Secrets</UI.Breadcrumb.Page
                  ></UI.Breadcrumb.Item
                >
              {:else}
                <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                  ><RouterLink
                    href={sessionSecretsPath(workspace.id, session.id)}
                    >Secrets</RouterLink
                  ></UI.Breadcrumb.Item
                >
                {#if secretBreadcrumb !== null}<UI.Breadcrumb.Separator
                  /><UI.Breadcrumb.Item aria-current="page"
                    ><UI.Breadcrumb.Page>{secretBreadcrumb}</UI.Breadcrumb.Page
                    ></UI.Breadcrumb.Item
                  >{/if}
              {/if}
            {/if}
          </UI.Breadcrumb.List>
        </UI.Breadcrumb.Root>
        {#if session !== null}<SessionViewNavigation
            workspaceID={workspace.id}
            sessionID={session.id}
            items={sessionViews(workspace.id, session.id, "secrets")}
            name={session.name}
            onRename={(name) =>
              renameSession(
                runtime,
                workspace.id,
                currentRoute.sessionID,
                name,
                updateSessionName,
              )}
          />{/if}
    {/snippet}
    {#if sessionStatus === "checking"}<UI.Text
            as="p"
            tone="muted"
            aria-busy="true"
            aria-live="polite"
          >
            Loading chat...
          </UI.Text>
        {:else if sessionStatus === "unavailable"}<UI.Text as="p" tone="muted">
            This chat could not be loaded.
          </UI.Text>
          <UI.Button
            variant="default"
            type="button"
            onclick={() => {
              const signal = abortController?.signal
              if (signal !== undefined)
                void loadRoute(currentRoute, generation, signal)
            }}>Try again</UI.Button
          >
        {:else if session !== null && routeSignal !== null}<SessionSecretsPage
            {embedded}
            workspaceID={workspace.id}
            sessionID={session.id}
            route={currentRoute}
            {controller}
            actions={viewActions}
          />
    {/if}
    {#if removal !== null}
      <UI.ResourceRemoval noun="secret" name={removal.description} busy={controller.state.deleting} error={controller.state.error}
        onCancel={() => { removal = null; controller.state.error = "" }}
        onConfirm={() => { if (removal?.id === controller.state.active?.id) void controller.remove() }} />
    {/if}
  </UI.ResourcePage>
{/if}
