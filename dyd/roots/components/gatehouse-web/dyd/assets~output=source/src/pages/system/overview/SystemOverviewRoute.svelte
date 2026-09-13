<script lang="ts">
  import SystemAccessGate from "../../../components/SystemAccessGate.svelte"
  import SystemOverviewPage from "./SystemOverviewPage.svelte"
</script>

<SystemAccessGate
  >{#snippet children()}<SystemOverviewPage />{/snippet}</SystemAccessGate
>
