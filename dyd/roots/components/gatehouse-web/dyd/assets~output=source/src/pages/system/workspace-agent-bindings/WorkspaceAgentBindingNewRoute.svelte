<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceAgentBindingNewPage from "./WorkspaceAgentBindingNewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<WorkspaceAgentBindingNewPage
    />{/snippet}</SystemAccessGate
>
