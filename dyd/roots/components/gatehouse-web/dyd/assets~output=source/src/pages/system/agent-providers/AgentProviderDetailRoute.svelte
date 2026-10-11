<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import AgentProviderDetailPage from "./AgentProviderDetailPage.svelte"
  let {
    providerID,
    editingPage = false,
  }: { providerID: string; editingPage?: boolean } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}{#key `${providerID}:${editingPage}`}<AgentProviderDetailPage
        {providerID}
        {editingPage}
      />{/key}{/snippet}</SystemAccessGate
>
