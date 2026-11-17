<script lang="ts">
  import { Bot, UserRound, UsersRound } from "@lucide/svelte"
  import type { Attributes } from "./types"

  let {
    kind = "person",
    src,
    label,
    ...rest
  }: Attributes & {
    kind?: "person" | "agent" | "group"
    src?: string
    label?: string
  } = $props()
  let failedSource = $state<string>()
</script>

<span
  data-slot="avatar"
  data-kind={kind}
  role="img"
  aria-label={label ??
    (kind === "agent" ? "Agent" : kind === "group" ? "Group" : "Person")}
  class="flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted text-muted-foreground"
  {...rest}
>
  {#if src && src !== failedSource}
    <img
      {src}
      alt=""
      class="size-full object-cover"
      onerror={() => {
        failedSource = src
      }}
    />
  {:else if kind === "agent"}
    <Bot size={20} strokeWidth={1.75} aria-hidden="true" />
  {:else if kind === "group"}
    <UsersRound size={20} strokeWidth={1.75} aria-hidden="true" />
  {:else}
    <UserRound size={20} strokeWidth={1.75} aria-hidden="true" />
  {/if}
</span>
