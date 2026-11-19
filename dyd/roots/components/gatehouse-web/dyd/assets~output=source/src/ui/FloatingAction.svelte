<script lang="ts">
  import IconButton from "./IconButton.svelte"
  import type { ComponentProps } from "svelte"
  let {
    ...rest
  }: Omit<ComponentProps<typeof IconButton>, "variant" | "raised"> = $props()
</script>

<div class="absolute end-2 bottom-3 z-10 md:end-6">
  <IconButton variant="default" raised {...rest} />
</div>
