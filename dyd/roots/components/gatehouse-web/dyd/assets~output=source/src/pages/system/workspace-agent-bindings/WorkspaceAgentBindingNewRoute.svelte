<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceAgentBindingNewPage from "./WorkspaceAgentBindingNewPage.svelte"

  let { workspaceID }: { workspaceID: string } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}{#key workspaceID}<WorkspaceAgentBindingNewPage
        {workspaceID}
      />{/key}{/snippet}</SystemAccessGate
>
