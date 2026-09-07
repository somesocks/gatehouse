<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import AgentProvidersPage from "./AgentProvidersPage.svelte"

</script>

<SystemAccessGate>{#snippet children()}<AgentProvidersPage />{/snippet}</SystemAccessGate>
