<script lang="ts">
  import { PanelLeftOpen } from "@lucide/svelte"
  import { signOut } from "../app/auth"
  import { useRuntime } from "../app/runtime.svelte"
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

<SidebarPage.Root className="brand-system-frame">
  <SidebarPage.Sidebar>
    <RouterLink class="brand" href="/app/">Gatehouse</RouterLink>
    <nav class="sidebar-nav" aria-label="System navigation">
      <section class="sidebar-section">
        <h2>System</h2>
        <ul>
          <li>
            <RouterLink
              class={active === "overview" ? "active" : undefined}
              href="/app/system">Overview</RouterLink
            >
          </li>
          <li>
            <RouterLink
              class={active === "principals" ? "active" : undefined}
              href="/app/system/principals">Principals</RouterLink
            >
          </li>
          <li>
            <RouterLink
              class={active === "grants" ? "active" : undefined}
              href="/app/system/grants">System grants</RouterLink
            >
          </li>
          <li>
            <RouterLink
              class={active === "agent-providers" ? "active" : undefined}
              href="/app/system/agent-providers">Agent providers</RouterLink
            >
          </li>
          <li>
            <RouterLink
              class={active === "agent-models" ? "active" : undefined}
              href="/app/system/agent-models">Agent models</RouterLink
            >
          </li>
          <li>
            <RouterLink
              class={active === "storage-providers" ? "active" : undefined}
              href="/app/system/storage-providers">Storage providers</RouterLink
            >
          </li>
          <li>
            <RouterLink
              class={active === "workspace-agent-bindings"
                ? "active"
                : undefined}
              href="/app/system/workspace-agent-bindings"
              >Workspace agent bindings</RouterLink
            >
          </li>
          <li>
            <RouterLink
              class={active === "workspace-storage-bindings"
                ? "active"
                : undefined}
              href="/app/system/workspace-storage-bindings"
              >Workspace storage bindings</RouterLink
            >
          </li>
        </ul>
      </section>
    </nav>
    <div class="sidebar-footer">
      <span>{runtime.auth.state.claims?.principal.name ?? "User"}</span><button
        class="button is-small is-danger is-light"
        type="button"
        onclick={() => void logout()}>Log out</button
      >
    </div>
  </SidebarPage.Sidebar>
  <SidebarPage.Page>
    <SidebarPage.Header
      ><SidebarPage.Toggle
        ><button
          class="mobile-menu-trigger"
          type="button"
          aria-label="Open navigation menu"
          ><PanelLeftOpen
            size={20}
            strokeWidth={2}
            aria-hidden="true"
          /></button
        ></SidebarPage.Toggle
      >
      <h1 class="brand-workspace-breadcrumb">
        <RouterLink
          class="brand-workspace-breadcrumb-segment"
          href="/app/system"><span>System</span></RouterLink
        ><span class="brand-workspace-breadcrumb-separator" aria-hidden="true"
          >/</span
        ><span>{title}</span>
      </h1></SidebarPage.Header
    >
    <SidebarPage.Body>{@render children()}</SidebarPage.Body>
  </SidebarPage.Page>
</SidebarPage.Root>
