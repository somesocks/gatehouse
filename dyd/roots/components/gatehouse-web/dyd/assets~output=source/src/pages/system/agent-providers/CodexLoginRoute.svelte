<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import CodexLoginPage from "./CodexLoginPage.svelte"
  let { providerID }: { providerID: string } = $props()
</script>

<SystemAccessGate>
  {#snippet children()}{#key providerID}<CodexLoginPage
        {providerID}
      />{/key}{/snippet}
</SystemAccessGate>
