<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import SystemWorkspaceDetailPage from "./SystemWorkspaceDetailPage.svelte"

  let { workspaceID }: { workspaceID: string } = $props()
</script>

<SystemAccessGate>
  {#snippet children()}<SystemWorkspaceDetailPage {workspaceID} />{/snippet}
</SystemAccessGate>
