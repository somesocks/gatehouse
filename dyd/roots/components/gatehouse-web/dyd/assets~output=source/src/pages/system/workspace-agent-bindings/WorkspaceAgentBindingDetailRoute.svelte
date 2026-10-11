<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceAgentBindingDetailPage from "./WorkspaceAgentBindingDetailPage.svelte"
  let {
    workspaceID,
    bindingID,
    editingPage = false,
  }: {
    workspaceID: string
    bindingID: string
    editingPage?: boolean
  } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}<WorkspaceAgentBindingDetailPage
      {workspaceID}
      {bindingID}
      {editingPage}
    />{/snippet}</SystemAccessGate
>
