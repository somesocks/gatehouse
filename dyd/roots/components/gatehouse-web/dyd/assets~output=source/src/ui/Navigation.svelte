<script lang="ts">
  import {
    Bot,
    Building2,
    Cpu,
    Database,
    FolderKanban,
    HardDrive,
    House,
    MessagesSquare,
    Network,
    ShieldCheck,
    UserRound,
    UsersRound,
  } from "@lucide/svelte"
  import * as Sidebar from "../lib/vendor/shadcn/sidebar"
  import RouterLink from "./RouterLink.svelte"
  import Heading from "./Heading.svelte"
  import WorkspaceSwitcher from "./WorkspaceSwitcher.svelte"
  import SidebarUser from "./SidebarUser.svelte"
  import type { NavigationIcon, NavigationOptions } from "./navigation"
  let {
    title,
    sections,
    workspaceSwitcher,
    user,
    onSignOut,
  }: NavigationOptions = $props()

  const icons = {
    overview: House,
    chats: MessagesSquare,
    projects: FolderKanban,
    groups: UsersRound,
    principals: UserRound,
    grants: ShieldCheck,
    "agent-providers": Bot,
    "agent-models": Cpu,
    "storage-providers": Database,
    workspaces: Building2,
    "workspace-agent-bindings": Network,
    "workspace-storage-bindings": HardDrive,
  } satisfies Record<NavigationIcon, unknown>
</script>

<Sidebar.Header>
  {#if workspaceSwitcher}
    <WorkspaceSwitcher {...workspaceSwitcher} />
  {:else if title}
    <Heading level={3}>{title}</Heading>
  {/if}
</Sidebar.Header>
<Sidebar.Content>
  {#each sections as section (section.title ?? "navigation")}
    <Sidebar.Group>
      {#if section.title}<Sidebar.GroupLabel>{section.title}</Sidebar.GroupLabel
        >{/if}
      <Sidebar.GroupContent>
        <nav
          aria-label={section.title
            ? `${section.title} navigation`
            : `${title ?? "Application"} navigation`}
        >
          <Sidebar.Menu>
            {#each section.items as item (item.href)}
              {@const Icon = icons[item.icon]}
              <Sidebar.MenuItem>
                <Sidebar.MenuButton
                  isActive={item.active}
                  class="data-active:bg-transparent data-active:text-sidebar-foreground hover:bg-transparent"
                >
                  {#snippet child({ props })}
                    <RouterLink
                      {...props}
                      href={item.href}
                      variant="navigation"
                      aria-current={item.active ? "page" : undefined}
                      target={item.external ? "_blank" : undefined}
                      rel={item.external ? "noopener noreferrer" : undefined}
                      ><Icon
                        class="size-4 shrink-0"
                        aria-hidden="true"
                      />{item.label}</RouterLink
                    >
                  {/snippet}
                </Sidebar.MenuButton>
              </Sidebar.MenuItem>
            {/each}
          </Sidebar.Menu>
        </nav>
      </Sidebar.GroupContent>
    </Sidebar.Group>
  {/each}
</Sidebar.Content>
<Sidebar.Footer>
  {#if user && onSignOut}<SidebarUser {user} {onSignOut} />{/if}
</Sidebar.Footer>
