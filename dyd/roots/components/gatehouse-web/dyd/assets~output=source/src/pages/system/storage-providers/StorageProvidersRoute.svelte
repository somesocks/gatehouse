<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import StorageProvidersPage from "./StorageProvidersPage.svelte"

</script>

<SystemAccessGate>{#snippet children()}<StorageProvidersPage />{/snippet}</SystemAccessGate>
