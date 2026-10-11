<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceStorageBindingListPage from "./WorkspaceStorageBindingListPage.svelte"

  let { workspaceID }: { workspaceID: string } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}{#key workspaceID}<WorkspaceStorageBindingListPage
        {workspaceID}
      />{/key}{/snippet}</SystemAccessGate
>
