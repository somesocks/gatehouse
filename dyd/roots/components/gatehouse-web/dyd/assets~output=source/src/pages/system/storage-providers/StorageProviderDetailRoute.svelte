<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import StorageProviderDetailPage from "./StorageProviderDetailPage.svelte"
  let { providerID }: { providerID: string } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}<StorageProviderDetailPage
      {providerID}
    />{/snippet}</SystemAccessGate
>
