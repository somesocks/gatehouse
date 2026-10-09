<script lang="ts">
  import { PageBody, PageHeading } from "../../ui"
  import * as UI from "../../ui"
  import { untrack } from "svelte"
  import type { ActivityClient } from "../../app/activity"
  import type { Workspace } from "../../app/access"
  import { createGroupsController } from "./groups-controller.svelte"

  let {
    workspace,
    signal,
    activity,
    onAuthenticationLost,
  }: {
    workspace: Workspace
    signal: AbortSignal
    activity: ActivityClient
    onAuthenticationLost: () => void
  } = $props()
  const controller = untrack(() =>
    createGroupsController({ activity, onAuthenticationLost }),
  )

  $effect(() => controller.start(workspace.id, signal))

  function ordered<T extends { id: string }>(items: T[]) {
    return [...items].sort((left, right) => right.id.localeCompare(left.id))
  }

  function matchesSearch(item: { id: string; name?: string }, search: string) {
    const query = search.trim().toLocaleLowerCase()
    return (
      query === "" ||
      item.id.toLocaleLowerCase().includes(query) ||
      item.name?.toLocaleLowerCase().includes(query) === true
    )
  }
</script>

<PageBody>
  <PageHeading>
    <UI.Heading level={1}>Groups</UI.Heading>
  </PageHeading>
  <UI.Field>
    <UI.Label layout="field"
      ><UI.Text>Search groups</UI.Text><UI.Input
        type="search"
        autocomplete="off"
        placeholder="Search groups"
        bind:value={controller.state.search}
      /></UI.Label
    >
  </UI.Field>
  <UI.List>
    {#each ordered(controller.state.groups.filter( (group) => matchesSearch(group, controller.state.search), )) as group}
      <UI.Item>
        <UI.Text>{group.name ?? "New Group"}</UI.Text><UI.Text
          as="small"
          size="small">{group.id}</UI.Text
        >
      </UI.Item>
    {:else}<UI.Notice>No groups match your search.</UI.Notice>{/each}
  </UI.List>
</PageBody>
