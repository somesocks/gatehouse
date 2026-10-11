<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import WorkspaceStorageBindingNewPage from "./WorkspaceStorageBindingNewPage.svelte"

  let { workspaceID }: { workspaceID: string } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}{#key workspaceID}<WorkspaceStorageBindingNewPage
        {workspaceID}
      />{/key}{/snippet}</SystemAccessGate
>
