<script lang="ts">
  import * as InputGroup from "../lib/vendor/shadcn/input-group"
  import type { HTMLFormAttributes } from "svelte/elements"
  let { children, ...rest }: Omit<HTMLFormAttributes, "class" | "style"> =
    $props()
</script>

<InputGroup.Root class="h-auto"
  ><form class="flex w-full min-w-0 flex-col gap-2 p-2" {...rest}>
    {@render children?.()}
  </form></InputGroup.Root
>
