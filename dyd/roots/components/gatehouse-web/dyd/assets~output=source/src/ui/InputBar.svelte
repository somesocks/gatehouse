<script lang="ts">
  import type { HTMLFormAttributes } from "svelte/elements"
  let { children, ...rest }: Omit<HTMLFormAttributes, "class" | "style"> =
    $props()
</script>

<form
  class="flex w-full min-w-0 flex-col gap-2 rounded-xl bg-card p-2"
  {...rest}
>
  {@render children?.()}
</form>
