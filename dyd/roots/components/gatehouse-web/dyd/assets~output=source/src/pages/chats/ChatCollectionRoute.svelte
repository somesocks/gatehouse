<script lang="ts">
  import { untrack } from "svelte"
  import { RouterLink, StatusPage } from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import * as UI from "../../ui"
  import { createChat, createProjectChat, type Chat } from "../../app/chats"
  import { useRuntime } from "../../app/runtime.svelte"
  import * as SidebarPage from "../../ui/sidebar-page"

  import type { Route } from "../../route"
  import ChatCollectionPage from "./ChatCollectionPage.svelte"

  type ChatCollectionRoute = Extract<Route, { kind: "session-collection" }>

  const runtime = useRuntime()
  const { access, auth } = runtime
  let generation = 0
  let creating = $state(false)
  let error = $state("")
  let abortController: AbortController | null = null
  let routeSignal = $state<AbortSignal | null>(null)

  const currentRoute = $derived(runtime.state.route as ChatCollectionRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`
  const chatsPath = (workspaceID: string) => `${workspacePath(workspaceID)}/ses`
  const isCurrent = (value: number, workspaceID: string, signal: AbortSignal) =>
    value === generation &&
    !signal.aborted &&
    currentRoute.workspaceID === workspaceID

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

  function activate(route: ChatCollectionRoute): () => void {
    const value = ++generation
    creating = false
    error = ""
    abortController?.abort()
    abortController = new AbortController()
    routeSignal = abortController.signal
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    )
      validateWorkspace(route, value, abortController.signal)
    else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    )
      void runtime.refresh()
    return () => {
      if (value === generation) abortController?.abort()
    }
  }

  function validateWorkspace(
    route: ChatCollectionRoute,
    value: number,
    signal: AbortSignal,
  ): void {
    if (
      access.state.workspaces.some(
        (candidate) => candidate.id === route.workspaceID,
      )
    )
      return
    if (isCurrent(value, route.workspaceID, signal))
      runtime.navigate(
        access.state.workspaces.length === 0
          ? "/app/no-access"
          : workspacePath(access.state.workspaces[0].id),
        true,
      )
  }

  async function create(): Promise<void> {
    if (creating) return
    const route = currentRoute
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    creating = true
    error = ""
    try {
      const response = route.projectID
        ? await createProjectChat(route.workspaceID, route.projectID, signal)
        : await createChat(route.workspaceID, signal)
      if (!isCurrent(value, route.workspaceID, signal)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("chat could not be created")
      const chat = (await response.json()) as Chat
      if (!isCurrent(value, route.workspaceID, signal)) return
      runtime.navigate(
        `${chatsPath(route.workspaceID)}/${encodeURIComponent(chat.id)}`,
      )
    } catch {
      if (isCurrent(value, route.workspaceID, signal))
        error = "Chat could not be created. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, signal)) creating = false
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
  <SidebarPage.Root
    ><SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "chats")}
      /></SidebarPage.Sidebar
    ><SidebarPage.Page
      ><SidebarPage.Header
        ><SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb"
          ><UI.Breadcrumb.List
            ><UI.Breadcrumb.Item
              ><RouterLink href={workspacePath(workspace.id)}
                >{workspace.name ?? "Workspace"}</RouterLink
              ></UI.Breadcrumb.Item
            ><UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
              ><UI.Breadcrumb.Page>Chats</UI.Breadcrumb.Page
              ></UI.Breadcrumb.Item
            ></UI.Breadcrumb.List
          ></UI.Breadcrumb.Root
        ></SidebarPage.Header
      ><SidebarPage.Body
        >{#if error}<UI.Notice variant="destructive" role="alert"
            >{error}</UI.Notice
          >{/if}
        {#if routeSignal !== null}<ChatCollectionPage
            {workspace}
            search={currentRoute.search}
            projectID={currentRoute.projectID}
            {creating}
            signal={routeSignal}
            onAuthenticationLost={() => runtime.requireLogin()}
            onCreate={create}
            onNavigate={runtime.navigate}
          />{/if}</SidebarPage.Body
      ></SidebarPage.Page
    ></SidebarPage.Root
  >
{/if}
