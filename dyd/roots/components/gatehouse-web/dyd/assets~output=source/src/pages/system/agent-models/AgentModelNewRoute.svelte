<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import AgentModelNewPage from "./AgentModelNewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<AgentModelNewPage />{/snippet}</SystemAccessGate
>
