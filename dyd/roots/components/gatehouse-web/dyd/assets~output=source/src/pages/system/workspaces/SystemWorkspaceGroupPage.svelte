<script lang="ts">
  import GroupPage from "../../groups/GroupPage.svelte"
  import SystemWorkspaceFrame from "./SystemWorkspaceFrame.svelte"
  let { workspaceID, groupID }: { workspaceID: string; groupID: string } =
    $props()
</script>

<SystemWorkspaceFrame
  {workspaceID}
  breadcrumbs={[
    {
      label: "Groups",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}/groups`,
    },
    { label: "Group" },
  ]}
>
  <GroupPage {workspaceID} {groupID} scope="system" />
</SystemWorkspaceFrame>
