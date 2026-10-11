<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import SystemGrantNewPage from "./SystemGrantNewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<SystemGrantNewPage />{/snippet}</SystemAccessGate
>
