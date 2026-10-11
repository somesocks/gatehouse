<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceAgentBindingListPage from "./WorkspaceAgentBindingListPage.svelte"

  let { workspaceID }: { workspaceID: string } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}{#key workspaceID}<WorkspaceAgentBindingListPage
        {workspaceID}
      />{/key}{/snippet}</SystemAccessGate
>
