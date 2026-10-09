<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceAgentBindingListPage from "./WorkspaceAgentBindingListPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<WorkspaceAgentBindingListPage
    />{/snippet}</SystemAccessGate
>
