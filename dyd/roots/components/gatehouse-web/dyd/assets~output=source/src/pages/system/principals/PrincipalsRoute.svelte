<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import PrincipalsPage from "./PrincipalsPage.svelte"
</script>

<SystemAccessGate>{#snippet children()}<PrincipalsPage />{/snippet}</SystemAccessGate>
