<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import AgentModelListPage from "./AgentModelListPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<AgentModelListPage />{/snippet}</SystemAccessGate
>
