<script lang="ts">
  import * as Card from "../lib/vendor/shadcn/card"
  import type { Attributes } from "./types"
  let { children, ...rest }: Attributes = $props()
</script>

<Card.Root {...rest}
  ><Card.Content
    ><div class="flex min-w-0 flex-col gap-4">
      {@render children?.()}
    </div></Card.Content
  ></Card.Root
>
