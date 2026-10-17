<script lang="ts">
  import { Tabs } from "bits-ui"
  import type { Snippet } from "svelte"

  let {
    value = $bindable(""),
    label,
    items,
    children,
  }: {
    value?: string
    label: string
    items: { value: string; label: string }[]
    children: Snippet<[string]>
  } = $props()
</script>

<Tabs.Root bind:value class="min-w-0">
  <Tabs.List
    aria-label={label}
    class="bg-muted text-muted-foreground inline-flex h-10 w-full items-center gap-1 rounded-lg p-1"
  >
    {#each items as item (item.value)}
      <Tabs.Trigger
        value={item.value}
        class="data-[state=active]:bg-background data-[state=active]:text-foreground focus-visible:ring-ring flex h-8 flex-1 items-center justify-center rounded-md px-4 text-sm font-medium outline-none focus-visible:ring-2 data-[state=active]:shadow-sm"
        >{item.label}</Tabs.Trigger
      >
    {/each}
  </Tabs.List>
  {#each items as item (item.value)}
    <Tabs.Content
      value={item.value}
      class="focus-visible:ring-ring mt-4 min-w-0 rounded-md outline-none focus-visible:ring-2"
    >
      {@render children(item.value)}
    </Tabs.Content>
  {/each}
</Tabs.Root>
