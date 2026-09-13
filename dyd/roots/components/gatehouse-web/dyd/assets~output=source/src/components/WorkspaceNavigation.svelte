<script lang="ts">
  import { signOut } from "../app/auth"
  import type { Workspace } from "../app/access"
  import { useRuntime } from "../app/runtime.svelte"
  import RouterLink from "./RouterLink.svelte"

  let {
    workspace,
    active,
  }: { workspace: Workspace; active?: "chats" | "projects" | "groups" } =
    $props()
  const runtime = useRuntime()
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`

  async function logout(): Promise<void> {
    try {
      await signOut()
    } finally {
      runtime.requireLogin()
    }
  }
</script>

<RouterLink class="workspace-navigation-wordmark" href="/app/"
  >Gatehouse</RouterLink
>
<div class="workspace-navigation-switcher">
  <label for="workspace">Workspace</label>
  <select
    id="workspace"
    value={workspace.id}
    onchange={(event) =>
      runtime.navigate(workspacePath(event.currentTarget.value))}
  >
    {#each runtime.access.state.workspaces as candidate (candidate.id)}<option
        value={candidate.id}>{candidate.name ?? candidate.id}</option
      >{/each}
  </select>
</div>
<nav class="workspace-navigation-links" aria-label="Workspace navigation">
  <ul>
    <li>
      <RouterLink
        class={active === "chats" ? "active" : undefined}
        href={`${workspacePath(workspace.id)}/ses`}>Chats</RouterLink
      >
    </li>
    <li>
      <RouterLink
        class={active === "projects" ? "active" : undefined}
        href={`${workspacePath(workspace.id)}/prj`}>Projects</RouterLink
      >
    </li>
    <li>
      <RouterLink
        class={active === "groups" ? "active" : undefined}
        href={`${workspacePath(workspace.id)}/grp`}>Groups</RouterLink
      >
    </li>
  </ul>
</nav>
{#if runtime.access.state.systemAccess === "available"}<div
    class="workspace-navigation-system"
  >
    <RouterLink href="/app/system" target="_blank" rel="noopener"
      >System</RouterLink
    >
  </div>{/if}
<div class="workspace-navigation-footer">
  <span>{runtime.auth.state.claims?.principal.name ?? "User"}</span><button
    class="brand-button brand-button--compact brand-button--danger"
    type="button"
    onclick={() => void logout()}>Log out</button
  >
</div>
