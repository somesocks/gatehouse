<script lang="ts">
  import { RouterLink } from "../../ui"
  import * as UI from "../../ui"
  import ProjectPicker from "../projects/ProjectPicker.svelte"
  import { untrack } from "svelte"
  import type { Workspace } from "../../app/access"
  import { createChatCollectionController } from "./chat-collection-controller.svelte"

  let {
    workspace,
    search,
    projectID = "",
    signal,
    onAuthenticationLost,
    onCreate,
    onNavigate,
  }: {
    workspace: Workspace
    search: string
    projectID?: string
    signal: AbortSignal
    onAuthenticationLost: () => void
    onCreate: () => void | Promise<void>
    onNavigate: (path: string) => void
  } = $props()
  const controller = untrack(() =>
    createChatCollectionController({ onAuthenticationLost }),
  )

  $effect(() => {
    const workspaceID = workspace.id
    const routeSearch = search
    const routeProject = projectID
    const routeSignal = signal
    return untrack(() =>
      controller.start(workspaceID, routeSearch, routeSignal, routeProject),
    )
  })

  function sessionsPath() {
    return `/app/wsp/${encodeURIComponent(workspace.id)}/ses`
  }

  function sessionPath(id: string) {
    return `${sessionsPath()}/${encodeURIComponent(id)}`
  }

  function searchPath() {
    const query = controller.state.search.trim()
    const params = new URLSearchParams()
    if (query) params.set("name", query)
    if (controller.state.projectID)
      params.set("project", controller.state.projectID)
    return params.size ? `${sessionsPath()}?${params}` : sessionsPath()
  }

  function createdAtLabel(value: string) {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) {
      return value
    }
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }
</script>

{#snippet chatActions()}<UI.Button
    variant="default"
    size="sm"
    type="button"
    onclick={() => void onCreate()}>New chat</UI.Button
  >{/snippet}
<UI.AdminContent>
  <UI.AdminHeader
    title="Chats"
    description="Conversations in this workspace."
    actions={chatActions}
  />
  <UI.SearchToolbar
    label="Search chats"
    placeholder="Chat name"
    bind:value={controller.state.search}
    loading={controller.state.loading}
    onsearch={() => onNavigate(searchPath())}
  >
    {#snippet filters()}<ProjectPicker
        workspaceID={workspace.id}
        compact
        placeholder="All projects"
        bind:value={controller.state.projectID}
      />{/snippet}
  </UI.SearchToolbar>
  {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
      >{controller.state.error}</UI.Notice
    >{/if}
  <UI.RecordTable headers={["Name", "Project", "Created"]}>
    {#each controller.state.chats as chat (chat.id)}
      <UI.RecordRow>
        <UI.RecordCell primary
          ><RouterLink href={sessionPath(chat.id)}
            >{chat.name ?? "Untitled chat"}</RouterLink
          ></UI.RecordCell
        >
        <UI.RecordCell
          >{#if chat.project}<RouterLink
              href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj/${encodeURIComponent(chat.project.id)}`}
              >{chat.project.name ?? "Untitled project"}</RouterLink
            >{:else}—{/if}</UI.RecordCell
        >
        <UI.RecordCell
          ><UI.Text as="time" datetime={chat.created_at}
            >{createdAtLabel(chat.created_at)}</UI.Text
          ></UI.RecordCell
        >
      </UI.RecordRow>
    {:else}<UI.TableEmpty
        columns={3}
        message={controller.state.loading
          ? "Searching chats…"
          : "No chats match your search."}
      />{/each}
  </UI.RecordTable>
  <UI.PaginationFooter
    noun="chats"
    count={controller.state.chats.length}
    previous={controller.state.history.length > 0}
    next={controller.state.cursor !== null}
    loading={controller.state.loading}
    onprevious={() => controller.previous(workspace.id, search, signal)}
    onnext={() => controller.loadMore(workspace.id, search, signal)}
  />
</UI.AdminContent>
