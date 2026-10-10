<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import SystemWorkspacesPage from "./SystemWorkspacesPage.svelte"
</script>

<SystemAccessGate>
  {#snippet children()}<SystemWorkspacesPage />{/snippet}
</SystemAccessGate>
