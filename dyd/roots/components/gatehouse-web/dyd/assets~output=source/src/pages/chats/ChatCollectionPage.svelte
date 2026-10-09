<script lang="ts">
  import { Search, PageBody, PageHeading, RouterLink } from "../../ui"
  import * as UI from "../../ui"
  import { untrack } from "svelte"
  import type { Workspace } from "../../app/access"
  import { createChatCollectionController } from "./chat-collection-controller.svelte"

  let {
    workspace,
    search,
    signal,
    onAuthenticationLost,
    onCreate,
    onNavigate,
  }: {
    workspace: Workspace
    search: string
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
    return untrack(() => controller.start(workspaceID, routeSearch, signal))
  })

  function sessionsPath() {
    return `/app/wsp/${encodeURIComponent(workspace.id)}/ses`
  }

  function sessionPath(id: string) {
    return `${sessionsPath()}/${encodeURIComponent(id)}`
  }

  function searchPath() {
    const query = controller.state.search.trim()
    return query === ""
      ? sessionsPath()
      : `${sessionsPath()}?${new URLSearchParams({ name: query })}`
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
<PageBody fluid>
  <PageHeading actions={chatActions}>
    <UI.Heading level={1}>Chats</UI.Heading>
  </PageHeading>
  <UI.Form
    layout="search"
    onsubmit={(event) => {
      event.preventDefault()
      onNavigate(searchPath())
    }}
  >
    <UI.Label layout="field"
      ><UI.Text>Search chats</UI.Text><UI.Input
        type="search"
        autocomplete="off"
        placeholder="Search chats"
        bind:value={controller.state.search}
      /></UI.Label
    >
    <UI.Button
      variant="ghost"
      size="icon"
      type="submit"
      aria-label="Search chats"
      title="Search chats"
      ><Search size={20} strokeWidth={2} aria-hidden="true" /></UI.Button
    >
  </UI.Form>
  <UI.List>
    {#each controller.state.chats as chat}
      <RouterLink variant="item" href={sessionPath(chat.id)}
        ><UI.Stack as="span" gap="tight"
          ><UI.Text>{chat.name ?? "New Chat"}</UI.Text><UI.Row
            as="span"
            gap="small"
            ><UI.Text as="time" datetime={chat.created_at}
              >{createdAtLabel(chat.created_at)}</UI.Text
            >{#if chat.project !== undefined}<UI.Text aria-hidden="true"
                >/</UI.Text
              ><UI.Text>{chat.project.name ?? "New Project"}</UI.Text
              >{/if}</UI.Row
          ></UI.Stack
        ></RouterLink
      >
    {:else}<UI.Notice>
        {controller.state.loading
          ? "Searching chats..."
          : "No chats match your search."}
      </UI.Notice>{/each}
  </UI.List>
  {#if controller.state.cursor !== null}<UI.Button
      variant="outline"
      size="sm"
      type="button"
      disabled={controller.state.loading}
      onclick={() => controller.loadMore(workspace.id, search, signal)}
      >{controller.state.loading ? "Loading..." : "Show more"}</UI.Button
    >{/if}
</PageBody>
