<script lang="ts">
  import * as Sidebar from "../lib/vendor/shadcn/sidebar"
  import RouterLink from "./RouterLink.svelte"
  import Heading from "./Heading.svelte"
  import Text from "./Text.svelte"
  import { Button } from "../lib/vendor/shadcn/button"
  import type { NavigationOptions } from "./navigation"
  let { title, items, user, onSignOut }: NavigationOptions = $props()
</script>

<Sidebar.Header><Heading level={3}>{title}</Heading></Sidebar.Header>
<Sidebar.Content>
  <Sidebar.Group
    ><Sidebar.GroupContent>
      <nav aria-label={`${title} navigation`}>
        <Sidebar.Menu>
          {#each items as item (item.href)}
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={item.active}>
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
    </Sidebar.GroupContent></Sidebar.Group
  >
</Sidebar.Content>
<Sidebar.Footer>
  {#if user}<Text size="small">{user}</Text>{/if}
  {#if onSignOut}<Button
      variant="outline"
      size="sm"
      onclick={() => void onSignOut?.()}>Log out</Button
    >{/if}
</Sidebar.Footer>
