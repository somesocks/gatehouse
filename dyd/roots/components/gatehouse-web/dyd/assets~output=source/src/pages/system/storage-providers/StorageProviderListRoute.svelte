<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import StorageProviderListPage from "./StorageProviderListPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<StorageProviderListPage />{/snippet}</SystemAccessGate
>
