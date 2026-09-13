<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import WorkspaceStorageBindingNewPage from "./WorkspaceStorageBindingNewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<WorkspaceStorageBindingNewPage
    />{/snippet}</SystemAccessGate
>
