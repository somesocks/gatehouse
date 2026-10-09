<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import AgentModelDetailPage from "./AgentModelDetailPage.svelte"
  let { modelID }: { modelID: string } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}<AgentModelDetailPage
      {modelID}
    />{/snippet}</SystemAccessGate
>
