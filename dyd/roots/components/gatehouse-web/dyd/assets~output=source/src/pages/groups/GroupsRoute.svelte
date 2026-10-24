<script lang="ts">
  import { untrack } from "svelte"
  import { RouterLink, StatusPage } from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import * as SidebarPage from "../../ui/sidebar-page"

  import type { Route } from "../../route"
  import GroupsPage from "./GroupsPage.svelte"
  import GroupPage from "./GroupPage.svelte"

  type GroupsRoute = Extract<Route, { kind: "group-collection" | "group" }>

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let generation = 0
  let abortController: AbortController | null = null
  let routeSignal = $state<AbortSignal | null>(null)
  let collectionContext = $state("")

  const currentRoute = $derived(runtime.state.route as GroupsRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`

  const activation = $derived(
    JSON.stringify([
      currentRoute.workspaceID,
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
  const context = $derived(`${currentRoute.workspaceID}/${auth.state.claims?.principal.ref.id ?? ""}`)
  $effect(() => {
    if (currentRoute.kind === "group-collection") collectionContext = context
  })

  function activate(route: GroupsRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    routeSignal = abortController.signal
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    )
      loadRoute(route, value, abortController.signal)
    else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    )
      void runtime.refresh()
    return () => {
      if (value === generation) abortController?.abort()
    }
  }

  function loadRoute(
    route: GroupsRoute,
    value: number,
    signal: AbortSignal,
  ): void {
    if (
      access.state.workspaces.some(
        (candidate) => candidate.id === route.workspaceID,
      )
    )
      return
    if (
      value === generation &&
      !signal.aborted &&
      currentRoute.workspaceID === route.workspaceID
    ) {
      runtime.navigate(
        access.state.workspaces.length === 0
          ? "/app/no-access"
          : workspacePath(access.state.workspaces[0].id),
        true,
      )
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
    description="Ask an administrator to grant you workspace access."
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
  <SidebarPage.Root
    ><SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "groups")}
      /></SidebarPage.Sidebar
    ><SidebarPage.Page
      ><SidebarPage.Header
        ><SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb"
          ><UI.Breadcrumb.List
            ><UI.Breadcrumb.Item
              ><RouterLink href={workspacePath(workspace.id)}
                >{workspace.name ?? workspace.id}</RouterLink
              ></UI.Breadcrumb.Item
            ><UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
              aria-current={currentRoute.kind === "group-collection"
                ? "page"
                : undefined}
              >{#if currentRoute.kind === "group"}<RouterLink
                  href={`${workspacePath(workspace.id)}/grp`}>Groups</RouterLink
                >{:else}<UI.Breadcrumb.Page>Groups</UI.Breadcrumb.Page>{/if}
              ></UI.Breadcrumb.Item
            >{#if currentRoute.kind === "group"}<UI.Breadcrumb.Separator
              /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page>Group</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >{/if}</UI.Breadcrumb.List
          ></UI.Breadcrumb.Root
        ></SidebarPage.Header
      ><SidebarPage.Body
        >{#if routeSignal !== null}{#key context}
            {#if collectionContext === context}
              <UI.Group hidden={currentRoute.kind !== "group-collection"}>
                <GroupsPage
                  {workspace}
                  signal={routeSignal}
                  {activity}
                  onAuthenticationLost={() => runtime.requireLogin()}
                />
              </UI.Group>
            {/if}
            {#if currentRoute.kind === "group"}{#key currentRoute.groupID}<GroupPage
                workspaceID={workspace.id}
                groupID={currentRoute.groupID}
              />{/key}{/if}{/key}{/if}</SidebarPage.Body
      ></SidebarPage.Page
    ></SidebarPage.Root
  >
{/if}
