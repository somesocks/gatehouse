<script lang="ts">
  import { EllipsisVertical, LogOut } from "@lucide/svelte"
  import * as Menu from "./dropdown-menu"
  import DropdownTrigger from "../lib/vendor/shadcn/dropdown-menu/dropdown-menu-trigger.svelte"
  import * as Sidebar from "../lib/vendor/shadcn/sidebar"
  import type { SidebarUserOptions } from "./navigation"

  let { user, onSignOut }: SidebarUserOptions = $props()
  const sidebar = Sidebar.useSidebar()
  const initials = $derived(
    user.name
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part.charAt(0).toUpperCase())
      .join("") || "U",
  )
</script>

<Sidebar.Menu>
  <Sidebar.MenuItem>
    <Menu.Root>
      <DropdownTrigger aria-label={`User menu for ${user.name}`}>
        {#snippet child({ props })}
          <Sidebar.MenuButton
            size="lg"
            {...props}
            class="bg-transparent hover:bg-transparent active:bg-transparent data-active:bg-transparent data-open:bg-transparent data-open:hover:bg-transparent"
          >
            <span
              aria-hidden="true"
              class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-sm font-medium text-muted-foreground"
              >{initials}</span
            >
            <span
              class="flex min-w-0 flex-1 flex-col items-start gap-0.5 text-left leading-tight"
            >
              <span class="w-full truncate text-sm font-medium"
                >{user.name}</span
              >
            </span>
            <EllipsisVertical
              class="ms-auto size-4 shrink-0"
              aria-hidden="true"
            />
          </Sidebar.MenuButton>
        {/snippet}
      </DropdownTrigger>
      <Menu.Content
        class="min-w-56 rounded-lg"
        side={sidebar.isMobile ? "bottom" : "right"}
        align="end"
        sideOffset={4}
      >
        <Menu.Label class="p-0 font-normal">
          <span class="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
            <span
              aria-hidden="true"
              class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-sm font-medium text-muted-foreground"
              >{initials}</span
            >
            <span class="flex min-w-0 flex-1 flex-col leading-tight">
              <span class="truncate font-medium">{user.name}</span>
            </span>
          </span>
        </Menu.Label>
        <Menu.Separator />
        <Menu.Item onSelect={() => void onSignOut()}>
          <LogOut size={16} aria-hidden="true" />
          Log out
        </Menu.Item>
      </Menu.Content>
    </Menu.Root>
  </Sidebar.MenuItem>
</Sidebar.Menu>
