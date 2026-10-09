<script lang="ts">
  import * as Card from "../lib/vendor/shadcn/card"
  import type { Attributes } from "./types"
  let {
    tone = "default",
    children,
    ...rest
  }: Attributes & {
    tone?: "default" | "secondary" | "primary-tint"
  } = $props()
</script>

<Card.Root
  class={tone === "secondary"
    ? "bg-secondary text-secondary-foreground"
    : tone === "primary-tint"
      ? "bg-primary/10 text-foreground"
      : undefined}
  {...rest}
  ><Card.Content
    ><div class="flex min-w-0 flex-col gap-4">
      {@render children?.()}
    </div></Card.Content
  ></Card.Root
>
