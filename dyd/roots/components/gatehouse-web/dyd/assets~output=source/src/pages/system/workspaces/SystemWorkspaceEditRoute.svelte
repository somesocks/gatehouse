<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import SystemWorkspaceEditPage from "./SystemWorkspaceEditPage.svelte"

  let { workspaceID }: { workspaceID: string } = $props()
</script>

<SystemAccessGate>
  {#snippet children()}<SystemWorkspaceEditPage {workspaceID} />{/snippet}
</SystemAccessGate>
