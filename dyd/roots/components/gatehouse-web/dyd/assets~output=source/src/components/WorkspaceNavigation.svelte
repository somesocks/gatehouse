<script lang="ts">
  import { signOut } from "../app/auth"
  import type { Workspace } from "../app/access"
  import { useRuntime } from "../app/runtime.svelte"
  import RouterLink from "./RouterLink.svelte"
  import * as SidebarPage from "./sidebar-page"

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

<SidebarPage.SidebarHeader title={workspace.name ?? workspace.id} />
<SidebarPage.SidebarBody>
  <nav aria-label="Workspace navigation">
    <ul>
      <li>
        <RouterLink
          aria-current={active === "chats" ? "page" : undefined}
          href={`${workspacePath(workspace.id)}/ses`}>Chats</RouterLink
        >
      </li>
      <li>
        <RouterLink
          aria-current={active === "projects" ? "page" : undefined}
          href={`${workspacePath(workspace.id)}/prj`}>Projects</RouterLink
        >
      </li>
      <li>
        <RouterLink
          aria-current={active === "groups" ? "page" : undefined}
          href={`${workspacePath(workspace.id)}/grp`}>Groups</RouterLink
        >
      </li>
    </ul>
  </nav>
  {#if runtime.access.state.systemAccess === "available"}<nav aria-label="System">
      <ul>
        <li>
          <RouterLink href="/app/system" target="_blank" rel="noopener"
            >System</RouterLink
          >
        </li>
      </ul>
    </nav>{/if}
</SidebarPage.SidebarBody>
<SidebarPage.SidebarFooter>
  <span>{runtime.auth.state.claims?.principal.name ?? "User"}</span><button
    class="small"
    type="button"
    onclick={() => void logout()}>Log out</button
  >
</SidebarPage.SidebarFooter>
