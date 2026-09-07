<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import WorkspaceStorageBindingsPage from "./WorkspaceStorageBindingsPage.svelte"
</script>

<SystemAccessGate>{#snippet children()}<WorkspaceStorageBindingsPage />{/snippet}</SystemAccessGate>
