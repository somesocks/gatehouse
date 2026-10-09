<script lang="ts">
  import * as SidebarPage from "./sidebar-page"
  import * as Breadcrumb from "../lib/vendor/shadcn/breadcrumb"
  import Navigation from "./Navigation.svelte"
  import RouterLink from "./RouterLink.svelte"
  import PageBody from "./PageBody.svelte"
  import type { Snippet } from "svelte"
  import type { NavigationOptions, BreadcrumbItem } from "./navigation"
  let {
    navigation,
    breadcrumbs,
    children,
  }: {
    navigation: NavigationOptions
    breadcrumbs: BreadcrumbItem[]
    children: Snippet
  } = $props()
</script>

<SidebarPage.Root>
  <SidebarPage.Sidebar><Navigation {...navigation} /></SidebarPage.Sidebar>
  <SidebarPage.Page>
    <SidebarPage.Header
      ><SidebarPage.Toggle /><Breadcrumb.Root
        ><Breadcrumb.List>
          {#each breadcrumbs as crumb, index}
            {#if index > 0}<Breadcrumb.Separator />{/if}
            <Breadcrumb.Item
              >{#if crumb.href}<RouterLink href={crumb.href}
                  >{crumb.label}</RouterLink
                >{:else}<Breadcrumb.Page>{crumb.label}</Breadcrumb.Page
                >{/if}</Breadcrumb.Item
            >
          {/each}
        </Breadcrumb.List></Breadcrumb.Root
      ></SidebarPage.Header
    >
    <SidebarPage.Body
      ><PageBody fluid>{@render children()}</PageBody></SidebarPage.Body
    >
  </SidebarPage.Page>
</SidebarPage.Root>
