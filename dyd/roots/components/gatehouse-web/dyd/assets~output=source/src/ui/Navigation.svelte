<script lang="ts">
  import * as Sidebar from "../lib/vendor/shadcn/sidebar"
  import RouterLink from "./RouterLink.svelte"
  import Heading from "./Heading.svelte"
  import Text from "./Text.svelte"
  import { Button } from "../lib/vendor/shadcn/button"
  import WorkspaceSwitcher from "./WorkspaceSwitcher.svelte"
  import type { NavigationOptions } from "./navigation"
  let {
    title,
    sections,
    workspaceSwitcher,
    user,
    onSignOut,
  }: NavigationOptions = $props()
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
              <Sidebar.MenuItem>
                <Sidebar.MenuButton
                  isActive={item.active}
                  class="data-active:bg-transparent data-active:text-sidebar-primary hover:bg-transparent"
                >
                  {#snippet child({ props })}
                    <RouterLink
                      {...props}
                      href={item.href}
                      aria-current={item.active ? "page" : undefined}
                      target={item.external ? "_blank" : undefined}
                      rel={item.external ? "noopener noreferrer" : undefined}
                      >{item.label}</RouterLink
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
  {#if user}<Text size="small">{user}</Text>{/if}
  {#if onSignOut}<Button
      variant="outline"
      size="sm"
      onclick={() => void onSignOut?.()}>Log out</Button
    >{/if}
</Sidebar.Footer>
