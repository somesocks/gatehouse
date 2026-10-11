<script lang="ts">
  import Badge from "./Badge.svelte"
  let {
    enabled,
    active = "Enabled",
    inactive = "Disabled",
  }: { enabled: boolean; active?: string; inactive?: string } = $props()
</script>

<Badge variant={enabled ? "secondary" : "outline"}
  >{enabled ? active : inactive}</Badge
>
