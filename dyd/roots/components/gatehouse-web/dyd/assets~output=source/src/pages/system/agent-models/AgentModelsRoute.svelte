<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import AgentModelsPage from "./AgentModelsPage.svelte"

</script>

<SystemAccessGate>{#snippet children()}<AgentModelsPage />{/snippet}</SystemAccessGate>
