<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import SystemGrantsPage from "./SystemGrantsPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<SystemGrantsPage />{/snippet}</SystemAccessGate
>
