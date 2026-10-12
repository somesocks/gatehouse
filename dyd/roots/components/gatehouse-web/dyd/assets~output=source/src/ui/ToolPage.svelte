<script lang="ts">
  import type { Snippet } from "svelte"
  import Stack from "./Stack.svelte"
  let { children }: { children: Snippet } = $props()
</script>

<main class="min-h-svh w-full min-w-0 p-6">
  <Stack gap="large">{@render children()}</Stack>
</main>
