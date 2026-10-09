<script lang="ts">
  import { Check, ChevronsUpDown, GalleryVerticalEnd } from "@lucide/svelte"
  import * as Menu from "./dropdown-menu"
  import DropdownTrigger from "../lib/vendor/shadcn/dropdown-menu/dropdown-menu-trigger.svelte"
  import * as Sidebar from "../lib/vendor/shadcn/sidebar"
  import type { WorkspaceSwitcherOptions } from "./navigation"

  let {
    currentID,
    currentName,
    workspaces,
    onSelect,
  }: WorkspaceSwitcherOptions = $props()
</script>

<Sidebar.Menu>
  <Sidebar.MenuItem>
    <Menu.Root>
      <DropdownTrigger aria-label={`Switch workspace, current ${currentName}`}>
        {#snippet child({ props })}
          <Sidebar.MenuButton size="lg" {...props}>
            <span
              class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground"
            >
              <GalleryVerticalEnd size={16} aria-hidden="true" />
            </span>
            <span
              class="flex min-w-0 flex-1 flex-col items-start gap-0.5 text-left leading-none"
            >
              <span class="w-full truncate text-sm font-medium"
                >{currentName}</span
              >
            </span>
            <ChevronsUpDown
              class="ms-auto size-4 shrink-0"
              aria-hidden="true"
            />
          </Sidebar.MenuButton>
        {/snippet}
      </DropdownTrigger>
      <Menu.Content align="start">
        {#each workspaces as workspace (workspace.id)}
          <Menu.Item onSelect={() => onSelect(workspace.id)}>
            <span class="min-w-0 flex-1 truncate">{workspace.name}</span>
            {#if workspace.id === currentID}<Check
                size={16}
                aria-hidden="true"
              />{/if}
          </Menu.Item>
        {/each}
      </Menu.Content>
    </Menu.Root>
  </Sidebar.MenuItem>
</Sidebar.Menu>
