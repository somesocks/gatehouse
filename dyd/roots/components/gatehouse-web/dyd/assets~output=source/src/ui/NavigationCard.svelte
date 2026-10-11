<script lang="ts">
  import { ChevronRight } from "@lucide/svelte"
  import RouterLink from "./RouterLink.svelte"
  let {
    title,
    description,
    href,
  }: { title: string; description: string; href: string } = $props()
</script>

<RouterLink variant="card" {href}>
  <div class="flex min-h-24 items-start justify-between gap-4">
    <div class="min-w-0 space-y-2">
      <h3 class="text-sm font-semibold">{title}</h3>
      <p class="text-muted-foreground text-sm leading-relaxed">{description}</p>
    </div>
    <ChevronRight
      class="text-muted-foreground mt-1 size-4 shrink-0"
      aria-hidden="true"
    />
  </div>
</RouterLink>
