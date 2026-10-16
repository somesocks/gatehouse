<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import SystemWorkspaceGroupsPage from "./SystemWorkspaceGroupsPage.svelte"
  import SystemWorkspaceGroupPage from "./SystemWorkspaceGroupPage.svelte"
  let { workspaceID, groupID }: { workspaceID: string; groupID?: string } =
    $props()
</script>

<SystemAccessGate
  >{#snippet children()}{#key `${workspaceID}/${groupID ?? ""}`}{#if groupID}<SystemWorkspaceGroupPage
          {workspaceID}
          {groupID}
        />{:else}<SystemWorkspaceGroupsPage
          {workspaceID}
        />{/if}{/key}{/snippet}</SystemAccessGate
>
