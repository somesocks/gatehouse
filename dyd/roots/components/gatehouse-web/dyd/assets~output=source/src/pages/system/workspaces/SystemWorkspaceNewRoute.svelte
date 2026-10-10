<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import SystemWorkspaceNewPage from "./SystemWorkspaceNewPage.svelte"
</script>

<SystemAccessGate>
  {#snippet children()}<SystemWorkspaceNewPage />{/snippet}
</SystemAccessGate>
