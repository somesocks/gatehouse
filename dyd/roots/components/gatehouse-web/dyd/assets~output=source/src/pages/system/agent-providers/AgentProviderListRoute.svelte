<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import AgentProviderListPage from "./AgentProviderListPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<AgentProviderListPage />{/snippet}</SystemAccessGate
>
