<script lang="ts">
  import SystemAccessGate from "../../../app/SystemAccessGate.svelte"
  import PrincipalDetailPage from "./PrincipalDetailPage.svelte"
  let {
    principalID,
    editingPage = false,
  }: { principalID: string; editingPage?: boolean } = $props()
</script>

<SystemAccessGate
  >{#snippet children()}<PrincipalDetailPage
      {principalID}
      {editingPage}
    />{/snippet}</SystemAccessGate
>
