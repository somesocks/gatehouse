<script lang="ts">
  import { untrack } from "svelte"
  import { Search } from "@lucide/svelte"
  import type { Workspace } from "../../app/access"
  import PageBody from "../../components/PageBody.svelte"
  import PageHeading from "../../components/PageHeading.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
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

<PageBody fluid>
  {#snippet chatActions()}<button
      class="primary small"
      type="button"
      onclick={() => void onCreate()}>New chat</button
    >{/snippet}
  <PageHeading actions={chatActions}>
    <h1>Chats</h1>
  </PageHeading>
  <form
    class="split"
    onsubmit={(event) => {
      event.preventDefault()
      onNavigate(searchPath())
    }}
  >
    <label class="field"
      ><span>Search chats</span><input
        type="search"
        autocomplete="off"
        placeholder="Search chats"
        bind:value={controller.state.search}
      /></label
    >
    <button
      class="icon"
      type="submit"
      aria-label="Search chats"
      title="Search chats"
      ><Search size={20} strokeWidth={2} aria-hidden="true" /></button
    >
  </form>
  <div class="list">
    {#each controller.state.chats as chat}
      <RouterLink class="list-item surface" href={sessionPath(chat.id)}
        ><span class="stack" style:--space="calc(var(--space) / 4)"
          ><span>{chat.name ?? "New Chat"}</span><span
            class="cluster"
            style:--space="calc(var(--space) / 2)"
            ><time datetime={chat.created_at}
              >{createdAtLabel(chat.created_at)}</time
            >{#if chat.project !== undefined}<span aria-hidden="true">/</span
              ><span>{chat.project.name ?? "New Project"}</span>{/if}</span
          ></span
        ></RouterLink
      >
    {:else}<p class="notice">
        {controller.state.loading
          ? "Searching chats..."
          : "No chats match your search."}
      </p>{/each}
  </div>
  {#if controller.state.cursor !== null}<button
      class="small"
      type="button"
      disabled={controller.state.loading}
      onclick={() => controller.loadMore(workspace.id, search, signal)}
      >{controller.state.loading ? "Loading..." : "Show more"}</button
    >{/if}
</PageBody>
