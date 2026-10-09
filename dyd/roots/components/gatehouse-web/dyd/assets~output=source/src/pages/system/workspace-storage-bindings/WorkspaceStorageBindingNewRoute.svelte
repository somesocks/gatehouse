<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceStorageBindingNewPage from "./WorkspaceStorageBindingNewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<WorkspaceStorageBindingNewPage
    />{/snippet}</SystemAccessGate
>
