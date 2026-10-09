<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceStorageBindingListPage from "./WorkspaceStorageBindingListPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<WorkspaceStorageBindingListPage
    />{/snippet}</SystemAccessGate
>
