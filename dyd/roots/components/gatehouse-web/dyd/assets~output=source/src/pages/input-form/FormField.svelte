<script lang="ts">
  import { X } from "@lucide/svelte"
  import SelectControl from "../../components/SelectControl.svelte"
  import FormField from "./FormField.svelte"
  import { initialListValue, patchDraft, type InputField } from "./form-data"

  let {
    field,
    path,
    value,
    label = field.label ?? "Value",
    controlID = `input-${path.join("-")}`,
    depth = 1,
    busy = false,
    onSet,
    onRemove,
  }: {
    field: InputField
    path: string[]
    value: unknown
    label?: string
    controlID?: string
    depth?: number
    busy?: boolean
    onSet: (path: string[], value: unknown) => Promise<boolean>
    onRemove: (path: string[]) => Promise<boolean>
  } = $props()

  const entries = $derived(Array.isArray(value) ? value : [])
  const headingTag = $derived(`h${Math.min(depth + 1, 6)}`)
  let addingEntry = $state(false)

  function isObject(value: unknown): value is Record<string, unknown> {
    return value !== null && typeof value === "object" && !Array.isArray(value)
  }

  function setEntry(index: number, childPath: string[], next: unknown, remove = false): Promise<boolean> {
    const segments = childPath.slice(path.length)
    if (remove && segments.length === 0)
      return onSet(path, entries.filter((_, position) => position !== index))
    const item = entries[index]
    const updated = segments.length === 0
      ? next
      : patchDraft(isObject(item) ? item : {}, segments, next, remove)
    return onSet(path, entries.map((entry, position) => position === index ? updated : entry))
  }
</script>

{#snippet fieldTitle()}
  {label}{#if !field.optional} <span aria-hidden="true">*</span><span class="visually-hidden"> required</span>{/if}
{/snippet}

{#if field.type === "object"}
  <section class="field-group input-object">
    <svelte:element this={headingTag} class="input-section-heading">{@render fieldTitle()}</svelte:element>
    <div class="field-group input-object-content">
      {#if field.optional && value === undefined}
        <button class="secondary small" type="button" disabled={busy} onclick={() => void onSet(path, {})}>Add {label}</button>
      {:else}
        {#if field.optional}<button class="secondary small" type="button" disabled={busy} onclick={() => void onRemove(path)}>Remove {label}</button>{/if}
        {#each field.fields ?? [] as child (child.id)}
          <FormField field={child} path={[...path, child.id ?? ""]} value={isObject(value) && Object.hasOwn(value, child.id ?? "") ? value[child.id ?? ""] : undefined} label={child.label ?? "Value"} controlID={`${controlID}-${child.id}`} depth={depth + 1} {busy} {onSet} {onRemove} />
        {/each}
      {/if}
    </div>
  </section>
{:else if field.type === "list"}
  <fieldset class="field-group input-list">
    <legend>{@render fieldTitle()}</legend>
    {#if field.optional && value === undefined}
      <button class="secondary small" type="button" disabled={busy} onclick={() => void onSet(path, [])}>Add {label}</button>
    {:else}
      {#each entries as entry, index (index)}
        <div class="input-list-entry">
          {#if field.item !== undefined}
            <FormField field={field.item} {path} value={entry} label={`Entry ${index + 1}`} controlID={`${controlID}-${index}`} {depth} {busy}
              onSet={(childPath, next) => setEntry(index, childPath, next)}
              onRemove={(childPath) => setEntry(index, childPath, undefined, true)} />
          {/if}
          <button class="secondary small" type="button" disabled={busy} onclick={() => void onSet(path, entries.filter((_, position) => position !== index))}>Remove entry {index + 1}</button>
        </div>
      {/each}
      {#if addingEntry && field.item !== undefined}
        <div class="input-list-entry">
          <FormField field={field.item} {path} value={undefined} label="New entry" controlID={`${controlID}-new`} {depth} {busy}
            onSet={async (_childPath, next) => {
              const saved = await onSet(path, [...entries, next])
              if (saved) addingEntry = false
              return saved
            }}
            onRemove={async () => { addingEntry = false; return true }} />
          <button class="secondary small" type="button" onclick={() => (addingEntry = false)}>Discard new entry</button>
        </div>
      {/if}
      <div class="cluster">
        <button class="secondary small" type="button" disabled={busy || addingEntry || field.item === undefined} onclick={() => {
          if (field.item === undefined) return
          if (field.item.type === "text" || field.item.type === "number" || field.item.type === "options") addingEntry = true
          else void onSet(path, [...entries, initialListValue(field.item)])
        }}>Add entry</button>
        {#if field.optional}<button class="secondary small" type="button" disabled={busy} onclick={() => void onRemove(path)}>Remove {label}</button>{/if}
      </div>
    {/if}
  </fieldset>
{:else}
  <div class="field input-field">
    {#if field.type === "boolean" && !field.optional}
      <label class="choice" for={controlID}>
        <input id={controlID} type="checkbox" checked={value === true} disabled={busy} onchange={(event) => void onSet(path, event.currentTarget.checked)} />
        {@render fieldTitle()}
      </label>
    {:else}
      <label for={controlID}>{@render fieldTitle()}</label>
      <div class="input-control" data-clearable={field.optional && value !== undefined || undefined}>
        {#if field.type === "boolean"}
          <SelectControl clearable={field.optional && value !== undefined}>
            <select id={controlID} disabled={busy} value={value === undefined ? "" : String(value)} onchange={(event) => {
              if (event.currentTarget.value === "") void onRemove(path)
              else void onSet(path, event.currentTarget.value === "true")
            }}>
              <option value="">Unanswered</option><option value="true">Yes</option><option value="false">No</option>
            </select>
          </SelectControl>
        {:else if field.type === "options"}
          <SelectControl clearable={field.optional && value !== undefined}>
            <select id={controlID} disabled={busy} value={typeof value === "string" ? value : ""} onchange={(event) => {
              if (event.currentTarget.value === "") void onRemove(path)
              else void onSet(path, event.currentTarget.value)
            }}>
              <option value="">Select an option</option>
              {#each field.choices ?? [] as choice (choice)}<option value={choice}>{choice}</option>{/each}
            </select>
          </SelectControl>
        {:else}
          <input id={controlID} type="text" inputmode={field.type === "number" ? "decimal" : undefined} value={typeof value === "string" ? value : ""} disabled={busy}
            onchange={(event) => void onSet(path, event.currentTarget.value)} />
        {/if}
        {#if field.optional && value !== undefined}
          <button class="icon inline input-clear" type="button" aria-label={`Clear ${label}`} title={`Clear ${label}`} disabled={busy} onclick={() => void onRemove(path)}><X size={16} strokeWidth={2} aria-hidden="true" /></button>
        {/if}
      </div>
    {/if}
  </div>
{/if}
