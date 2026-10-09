<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import AgentProviderDetailPage from "./AgentProviderDetailPage.svelte"
  let { providerID }: { providerID: string } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}<AgentProviderDetailPage
      {providerID}
    />{/snippet}</SystemAccessGate
>
