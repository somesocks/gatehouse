<script lang="ts">
  import { Spinner } from "../lib/vendor/shadcn/spinner"
  import type { Attributes } from "./types"
  let { children, ...rest }: Attributes = $props()
</script>

<span class="inline-flex items-center gap-2" {...rest}
  ><Spinner aria-hidden="true" />{@render children?.()}</span
>
