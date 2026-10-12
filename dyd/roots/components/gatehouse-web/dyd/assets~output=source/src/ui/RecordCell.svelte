<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    label,
    primary = false,
    actions = false,
    children,
  }: {
    label?: string
    primary?: boolean
    actions?: boolean
    children: Snippet
  } = $props()
</script>

<td
  class={actions
    ? "w-px px-5 py-4 text-right align-middle whitespace-nowrap"
    : primary
      ? "w-[32%] min-w-56 px-5 py-4 align-middle"
      : "px-5 py-4 align-middle last:text-right"}
>
  {#if label}<span class="sr-only">{label}: </span>{/if}
  <div
    class={primary
      ? "flex min-w-0 flex-col gap-1 [overflow-wrap:anywhere] [&>a]:line-clamp-2"
      : "min-w-0 [overflow-wrap:anywhere]"}
  >
    {@render children()}
  </div>
</td>
