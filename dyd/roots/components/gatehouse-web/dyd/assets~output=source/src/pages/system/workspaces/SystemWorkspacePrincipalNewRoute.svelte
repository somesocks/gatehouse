<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import SystemWorkspacePrincipalNewPage from "./SystemWorkspacePrincipalNewPage.svelte"

  let { workspaceID }: { workspaceID: string } = $props()
</script>

<SystemAccessGate>
  {#snippet children()}{#key workspaceID}<SystemWorkspacePrincipalNewPage
        {workspaceID}
      />{/key}{/snippet}
</SystemAccessGate>
