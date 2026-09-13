<script lang="ts">
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../app/activity"
  import type { Workspace } from "../../app/access"
  import { createGroupsController } from "./groups-controller.svelte"

  let { workspace, signal, activity, onAuthenticationLost }: { workspace: Workspace; signal: AbortSignal; activity: ActivityClient; onAuthenticationLost: () => void } = $props()
  const controller = untrack(() => createGroupsController({ activity, onAuthenticationLost }))

  $effect(() => controller.start(workspace.id, signal))

  function ordered<T extends { id: string }>(items: T[]) {
    return [...items].sort((left, right) => right.id.localeCompare(left.id))
  }

  function matchesSearch(item: { id: string; name?: string }, search: string) {
    const query = search.trim().toLocaleLowerCase()
    return query === "" || item.id.toLocaleLowerCase().includes(query) || item.name?.toLocaleLowerCase().includes(query) === true
  }
</script>

<section class="collection-page">
  <div class="collection-heading"><h1 class="brand-dashboard-title">Groups</h1></div>
  <div class="collection-search"><label><span>Search groups</span><input class="input" type="search" autocomplete="off" placeholder="Search groups" bind:value={controller.state.search} /></label></div>
  <div class="collection-list">
    {#each ordered(controller.state.groups.filter((group) => matchesSearch(group, controller.state.search))) as group}
      <div class="dashboard-row"><span>{group.name ?? "New Group"}</span><small>{group.id}</small></div>
    {:else}<p class="dashboard-empty">No groups match your search.</p>{/each}
  </div>
</section>
