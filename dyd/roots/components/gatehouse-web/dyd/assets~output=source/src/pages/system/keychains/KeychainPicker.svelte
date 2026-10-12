<script lang="ts">
  import * as UI from "../../../ui"
  import type { SystemKeychain } from "../../../app/system"
  // These are the configured keychain references already supplied by the
  // provider form's keychain API, rather than a database record collection.
  let {
    value = $bindable(""),
    keychains,
    loading = false,
    error = "",
    onreload,
  }: {
    value?: string
    keychains: SystemKeychain[]
    loading?: boolean
    error?: string
    onreload?: () => void
  } = $props()
  let search = $state("")
  let appliedSearch = $state("")
  let page = $state(0)
  const matches = $derived(
    keychains.filter((keychain) =>
      keychain.id.toLowerCase().includes(appliedSearch.toLowerCase()),
    ),
  )
  const items = $derived(matches.slice(page * 50, (page + 1) * 50))
  const selected = $derived(keychains.find((keychain) => keychain.id === value))
</script>

<UI.RecordPicker
  label="Keychain"
  {loading}
  {error}
  bind:value
  bind:search
  selectedLabel={selected?.id}
  items={items.map((keychain) => ({
    id: keychain.id,
    label: keychain.id,
    description: `Version ${keychain.version}`,
  }))}
  previous={page > 0}
  next={(page + 1) * 50 < matches.length}
  onsearch={() => {
    if (error) onreload?.()
    appliedSearch = search.trim()
    page = 0
  }}
  onprevious={() => {
    if (page > 0) page--
  }}
  onnext={() => {
    if ((page + 1) * 50 < matches.length) page++
  }}
/>
