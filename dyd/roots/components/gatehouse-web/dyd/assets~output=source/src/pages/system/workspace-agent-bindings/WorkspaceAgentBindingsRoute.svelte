<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import WorkspaceAgentBindingsPage from "./WorkspaceAgentBindingsPage.svelte"
</script>

<SystemAccessGate>{#snippet children()}<WorkspaceAgentBindingsPage />{/snippet}</SystemAccessGate>
