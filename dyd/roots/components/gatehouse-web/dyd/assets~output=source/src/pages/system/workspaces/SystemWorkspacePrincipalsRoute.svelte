<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import SystemWorkspacePrincipalsPage from "./SystemWorkspacePrincipalsPage.svelte"

  let { workspaceID }: { workspaceID: string } = $props()
</script>

<SystemAccessGate>
  {#snippet children()}{#key workspaceID}<SystemWorkspacePrincipalsPage
        {workspaceID}
      />{/key}{/snippet}
</SystemAccessGate>
