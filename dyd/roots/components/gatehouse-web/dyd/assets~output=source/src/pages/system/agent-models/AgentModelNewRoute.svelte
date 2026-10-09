<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import AgentModelNewPage from "./AgentModelNewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<AgentModelNewPage />{/snippet}</SystemAccessGate
>
