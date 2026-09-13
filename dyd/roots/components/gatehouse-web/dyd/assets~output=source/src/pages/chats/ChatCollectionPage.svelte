<script lang="ts">
  import { untrack } from "svelte"
  import { Search } from "@lucide/svelte"
  import type { Workspace } from "../../app/access"
  import RouterLink from "../../components/RouterLink.svelte"
  import { createChatCollectionController } from "./chat-collection-controller.svelte"

  let { workspace, search, signal, onAuthenticationLost, onCreate, onNavigate }: {
    workspace: Workspace
    search: string
    signal: AbortSignal
    onAuthenticationLost: () => void
    onCreate: () => void | Promise<void>
    onNavigate: (path: string) => void
  } = $props()
  const controller = untrack(() => createChatCollectionController({ onAuthenticationLost }))

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
    return query === "" ? sessionsPath() : `${sessionsPath()}?${new URLSearchParams({ name: query })}`
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

<section class="collection-page">
  <div class="collection-heading"><h1 class="brand-dashboard-title">Chats</h1><button class="button is-primary is-small" type="button" onclick={() => void onCreate()}>New chat</button></div>
  <form class="collection-search" onsubmit={(event) => { event.preventDefault(); onNavigate(searchPath()) }}>
    <label><span>Search chats</span><input class="input" type="search" autocomplete="off" placeholder="Search chats" bind:value={controller.state.search} /></label>
    <button class="button" type="submit" aria-label="Search chats" title="Search chats"><Search size={20} strokeWidth={2} aria-hidden="true" /></button>
  </form>
  <div class="collection-list">
    {#each controller.state.chats as chat}
      <RouterLink class="dashboard-row" href={sessionPath(chat.id)}><span class="dashboard-row-content"><span>{chat.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={chat.created_at}>{createdAtLabel(chat.created_at)}</time>{#if chat.project !== undefined}<span aria-hidden="true">/</span><span>{chat.project.name ?? "New Project"}</span>{/if}</span></span></RouterLink>
    {:else}<p class="dashboard-empty">{controller.state.loading ? "Searching chats..." : "No chats match your search."}</p>{/each}
  </div>
  {#if controller.state.cursor !== null}<button class="button is-small" type="button" disabled={controller.state.loading} onclick={() => controller.loadMore(workspace.id, search, signal)}>{controller.state.loading ? "Loading..." : "Show more"}</button>{/if}
</section>
