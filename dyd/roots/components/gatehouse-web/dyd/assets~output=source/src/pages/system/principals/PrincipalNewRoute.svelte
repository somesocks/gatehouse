<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import PrincipalNewPage from "./PrincipalNewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<PrincipalNewPage />{/snippet}</SystemAccessGate
>
