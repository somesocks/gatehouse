<script lang="ts">
  import { signOut } from "../app/auth"
  import { useRuntime } from "../app/runtime.svelte"
  import PageBody from "./PageBody.svelte"
  import RouterLink from "./RouterLink.svelte"
  import type { Snippet } from "svelte"
  import * as SidebarPage from "./sidebar-page"

  let {
    active,
    title,
    children,
  }: { active: string; title: string; children: Snippet } = $props()
  const runtime = useRuntime()

  async function logout(): Promise<void> {
    try {
      await signOut()
    } finally {
      runtime.requireLogin()
    }
  }
</script>

<SidebarPage.Root>
  <SidebarPage.Sidebar>
    <SidebarPage.SidebarHeader />
    <SidebarPage.SidebarBody>
      <nav aria-label="System navigation">
        <ul>
          <li>
            <RouterLink
              aria-current={active === "overview" ? "page" : undefined}
              href="/app/system">Overview</RouterLink
            >
          </li>
          <li>
            <RouterLink
              aria-current={active === "principals" ? "page" : undefined}
              href="/app/system/principals">Principals</RouterLink
            >
          </li>
          <li>
            <RouterLink
              aria-current={active === "grants" ? "page" : undefined}
              href="/app/system/grants">System grants</RouterLink
            >
          </li>
          <li>
            <RouterLink
              aria-current={active === "agent-providers" ? "page" : undefined}
              href="/app/system/agent-providers">Agent providers</RouterLink
            >
          </li>
          <li>
            <RouterLink
              aria-current={active === "agent-models" ? "page" : undefined}
              href="/app/system/agent-models">Agent models</RouterLink
            >
          </li>
          <li>
            <RouterLink
              aria-current={active === "storage-providers" ? "page" : undefined}
              href="/app/system/storage-providers">Storage providers</RouterLink
            >
          </li>
          <li>
            <RouterLink
              aria-current={active === "workspace-agent-bindings"
                ? "page"
                : undefined}
              href="/app/system/workspace-agent-bindings"
              >Workspace agent bindings</RouterLink
            >
          </li>
          <li>
            <RouterLink
              aria-current={active === "workspace-storage-bindings"
                ? "page"
                : undefined}
              href="/app/system/workspace-storage-bindings"
              >Workspace storage bindings</RouterLink
            >
          </li>
        </ul>
      </nav>
    </SidebarPage.SidebarBody>
    <SidebarPage.SidebarFooter>
      <span>{runtime.auth.state.claims?.principal.name ?? "User"}</span><button
        class="small"
        type="button"
        onclick={() => void logout()}>Log out</button
      >
    </SidebarPage.SidebarFooter>
  </SidebarPage.Sidebar>
  <SidebarPage.Page>
    <SidebarPage.Header>
      <SidebarPage.Toggle />
      <nav aria-label="Breadcrumb">
        <ol>
          <li><RouterLink href="/app/system">System</RouterLink></li>
          <li aria-current="page">{title}</li>
        </ol>
      </nav>
    </SidebarPage.Header>
    <SidebarPage.Body><PageBody fluid>{@render children()}</PageBody></SidebarPage.Body>
  </SidebarPage.Page>
</SidebarPage.Root>
