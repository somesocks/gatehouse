<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceStorageBindingDetailPage from "./WorkspaceStorageBindingDetailPage.svelte"
  let {
    workspaceID,
    providerID,
    editingPage = false,
  }: {
    workspaceID: string
    providerID: string
    editingPage?: boolean
  } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}<WorkspaceStorageBindingDetailPage
      {workspaceID}
      {providerID}
      {editingPage}
    />{/snippet}</SystemAccessGate
>
