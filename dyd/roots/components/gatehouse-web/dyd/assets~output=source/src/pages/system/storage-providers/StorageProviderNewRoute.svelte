<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import StorageProviderNewPage from "./StorageProviderNewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<StorageProviderNewPage />{/snippet}</SystemAccessGate
>
