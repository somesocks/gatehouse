<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import AgentProviderNewPage from "./AgentProviderNewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<AgentProviderNewPage />{/snippet}</SystemAccessGate
>
