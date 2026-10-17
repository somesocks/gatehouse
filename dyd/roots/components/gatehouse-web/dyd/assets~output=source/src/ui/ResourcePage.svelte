<script lang="ts">
  import type { Snippet } from "svelte"
  import * as SidebarPage from "./sidebar-page"

  let {
    embedded = false,
    sidebar,
    header,
    children,
  }: {
    embedded?: boolean
    sidebar: Snippet
    header: Snippet
    children: Snippet
  } = $props()
</script>

{#if embedded}
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
