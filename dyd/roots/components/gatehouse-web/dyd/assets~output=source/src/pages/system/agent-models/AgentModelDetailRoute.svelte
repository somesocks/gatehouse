<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import AgentModelDetailPage from "./AgentModelDetailPage.svelte"
  let {
    modelID,
    editingPage = false,
  }: { modelID: string; editingPage?: boolean } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}{#key `${modelID}:${editingPage}`}<AgentModelDetailPage
        {modelID}
        {editingPage}
      />{/key}{/snippet}</SystemAccessGate
>
