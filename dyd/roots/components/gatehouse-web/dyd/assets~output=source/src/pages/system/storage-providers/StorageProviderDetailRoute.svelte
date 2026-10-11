<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import StorageProviderDetailPage from "./StorageProviderDetailPage.svelte"
  let {
    providerID,
    editingPage = false,
  }: { providerID: string; editingPage?: boolean } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}{#key `${providerID}:${editingPage}`}<StorageProviderDetailPage
        {providerID}
        {editingPage}
      />{/key}{/snippet}</SystemAccessGate
>
