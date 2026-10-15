<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import CodexLoginPage from "./CodexLoginPage.svelte"
</script>

<SystemAccessGate>
  {#snippet children()}<CodexLoginPage />{/snippet}
</SystemAccessGate>
