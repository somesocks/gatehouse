<script lang="ts">
  import type { Snippet } from "svelte"
  import Stack from "./Stack.svelte"
  let { children }: { children: Snippet } = $props()
</script>

<main class="mx-auto min-h-svh w-full max-w-3xl p-6">
  <Stack gap="large">{@render children()}</Stack>
</main>
