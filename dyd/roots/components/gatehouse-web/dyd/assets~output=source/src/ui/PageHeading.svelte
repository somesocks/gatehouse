<script lang="ts">
  import type { Snippet } from "svelte"
  import Stack from "./Stack.svelte"
  import Row from "./Row.svelte"
  let {
    as = "header",
    actions,
    children,
  }: { as?: "div" | "header"; actions?: Snippet; children: Snippet } = $props()
</script>

<Row {as} justify="between" align="start" gap="normal">
  <Stack gap="tight">{@render children()}</Stack>
  {#if actions}<Row justify="end">{@render actions()}</Row>{/if}
</Row>
