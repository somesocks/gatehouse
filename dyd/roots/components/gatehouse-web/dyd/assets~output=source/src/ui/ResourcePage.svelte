<script lang="ts">
  import type { Snippet } from "svelte"
  import * as SidebarPage from "./sidebar-page"
  import ResourceDialog from "./ResourceDialog.svelte"
  import type { ResourceDialogOptions } from "./resource-dialog"

  let {
    embedded = false,
    dialog,
    sidebar,
    header,
    children,
  }: {
    embedded?: boolean
    dialog?: ResourceDialogOptions
    sidebar: Snippet
    header: Snippet
    children: Snippet
  } = $props()
</script>

{#if embedded && dialog}
  <ResourceDialog {...dialog}>{@render children()}</ResourceDialog>
{:else if embedded}
  {@render children()}
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar>{@render sidebar()}</SidebarPage.Sidebar>
    <SidebarPage.Page>
      <SidebarPage.Header>{@render header()}</SidebarPage.Header>
      <SidebarPage.Body>{@render children()}</SidebarPage.Body>
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
