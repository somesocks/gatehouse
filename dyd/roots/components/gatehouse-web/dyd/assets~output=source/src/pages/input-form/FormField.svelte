<script lang="ts">
  import { Paperclip, X } from "@lucide/svelte"
  import InputControl from "../../components/InputControl.svelte"
  import SelectControl from "../../components/SelectControl.svelte"
  import FormField from "./FormField.svelte"
  import { initialListValue, patchDraft, type InputField, type InputFileSummary } from "./form-data"

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
    onUpload,
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
    onUpload?: (path: string[], files: File[], save: (value: InputFileSummary) => Promise<boolean>) => Promise<boolean>
  } = $props()

  const entries = $derived(Array.isArray(value) ? value : [])
  const headingTag = $derived(`h${Math.min(depth + 1, 6)}`)
  const hasClearButton = $derived(field.type === "text" || field.type === "number")
  const clearable = $derived(hasClearButton && value !== undefined)
  let addingEntry = $state(false)
  let savingCount = $state(0)
  const saving = $derived(savingCount > 0)
  let inputElement = $state<HTMLInputElement | undefined>()
  let clearButton = $state<HTMLButtonElement | undefined>()
  let fileElement = $state<HTMLInputElement | undefined>()
  let fileError = $state("")

  async function uploadFiles(selected: File[]): Promise<void> {
    if (selected.length === 0 || onUpload === undefined) return
    const current: InputFileSummary[] = Array.isArray(value) ? [...value] : []
    if (field.max_files !== undefined && BigInt(current.length + selected.length) > BigInt(field.max_files)) {
      fileError = `Select no more than ${field.max_files} files.`
      return
    }
    fileError = ""
    savingCount += 1
    try {
      const invalid = selected.find((file) => field.media_types !== undefined && !field.media_types.some((accepted) =>
        accepted === file.type || accepted.endsWith("/*") && file.type.startsWith(accepted.slice(0, -1))))
      if (invalid !== undefined) {
        fileError = `${invalid.name} has an unsupported media type.`
        return
      }
      await onUpload(path, selected, async (summary) => {
        const next = [...current, summary]
        if (!(await onSet(path, next))) return false
        current.push(summary)
        return true
      })
    } finally {
      savingCount -= 1
    }
  }

  function removeFile(index: number): void {
    const current: InputFileSummary[] = Array.isArray(value) ? value : []
    const next = current.filter((_, at) => at !== index)
    if (next.length === 0) void saveRemove(path)
    else void saveSet(path, next)
  }

  function focusNextFormControl(button: HTMLButtonElement): void {
    const controls = Array.from(button.form?.elements ?? [])
    for (const next of controls.slice(controls.indexOf(button) + 1)) {
      if (next instanceof HTMLElement && next.tabIndex >= 0 && !next.matches(":disabled") && next.getClientRects().length > 0) {
        next.focus()
        return
      }
    }
  }

  $effect(() => {
    if ((saving || busy || !clearable) && clearButton === document.activeElement && clearButton !== undefined)
      focusNextFormControl(clearButton)
  })

  function clearAnswer(): void {
    if (busy || saving || !clearable) return
    inputElement?.focus()
    void saveRemove(path)
  }

  async function saveSet(path: string[], next: unknown): Promise<boolean> {
    savingCount += 1
    try {
      return await onSet(path, next)
    } finally {
      savingCount -= 1
    }
  }

  async function saveRemove(path: string[]): Promise<boolean> {
    savingCount += 1
    try {
      return await onRemove(path)
    } finally {
      savingCount -= 1
    }
  }

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

{#snippet clearAction()}
  <button bind:this={clearButton} class="icon inline input-clear" type="button" aria-label={`Clear ${label}`} title={`Clear ${label}`}
    data-empty={!clearable && !saving || undefined} aria-hidden={!clearable && !saving || undefined}
    aria-disabled={busy || saving || undefined} tabindex={busy || saving || !clearable ? -1 : 0}
    onfocus={(event) => { if (saving || busy || !clearable) focusNextFormControl(event.currentTarget) }}
    onclick={clearAnswer}><X size={16} strokeWidth={2} aria-hidden="true" /></button>
{/snippet}

{#if field.type === "object"}
  <section class="field-group input-object">
    <svelte:element this={headingTag} class="input-section-heading">{@render fieldTitle()}</svelte:element>
    <div class="field-group input-object-content">
      {#if field.optional && value === undefined}
        <div class="cluster">
          <button class="secondary small" type="button" disabled={busy} onclick={() => void saveSet(path, {})}>Add {label}</button>
          {#if saving}<span class="spinner input-action-spinner" role="status"><span class="visually-hidden">Saving {label}</span></span>{/if}
        </div>
      {:else}
        {#if field.optional}
          <div class="cluster">
            <button class="secondary small" type="button" disabled={busy} onclick={() => void saveRemove(path)}>Remove {label}</button>
            {#if saving}<span class="spinner input-action-spinner" role="status"><span class="visually-hidden">Saving {label}</span></span>{/if}
          </div>
        {/if}
        {#each field.fields ?? [] as child (child.id)}
          <FormField field={child} path={[...path, child.id ?? ""]} value={isObject(value) && Object.hasOwn(value, child.id ?? "") ? value[child.id ?? ""] : undefined} label={child.label ?? "Value"} controlID={`${controlID}-${child.id}`} depth={depth + 1} {busy} {onSet} {onRemove} {onUpload} />
        {/each}
      {/if}
    </div>
  </section>
{:else if field.type === "list"}
  <fieldset class="field-group input-list">
    <legend>{@render fieldTitle()}{#if saving} <span class="spinner input-action-spinner" role="status"><span class="visually-hidden">Saving {label}</span></span>{/if}</legend>
    {#if field.optional && value === undefined}
      <button class="secondary small" type="button" disabled={busy} onclick={() => void saveSet(path, [])}>Add {label}</button>
    {:else}
      {#each entries as entry, index (index)}
        <div class="input-list-entry">
          {#if field.item !== undefined}
            <FormField field={field.item} {path} value={entry} label={`Entry ${index + 1}`} controlID={`${controlID}-${index}`} {depth} {busy} {onUpload}
              onSet={(childPath, next) => setEntry(index, childPath, next)}
              onRemove={(childPath) => setEntry(index, childPath, undefined, true)} />
          {/if}
          <button class="secondary small" type="button" disabled={busy} onclick={() => void saveSet(path, entries.filter((_, position) => position !== index))}>Remove entry {index + 1}</button>
        </div>
      {/each}
      {#if addingEntry && field.item !== undefined}
        <div class="input-list-entry">
          <FormField field={field.item} {path} value={undefined} label="New entry" controlID={`${controlID}-new`} {depth} {busy} {onUpload}
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
          if (field.item.type === "text" || field.item.type === "number" || field.item.type === "options" || field.item.type === "files") addingEntry = true
          else void saveSet(path, [...entries, initialListValue(field.item)])
        }}>Add entry</button>
        {#if field.optional}<button class="secondary small" type="button" disabled={busy} onclick={() => void saveRemove(path)}>Remove {label}</button>{/if}
      </div>
    {/if}
  </fieldset>
{:else}
  <div class="field input-field">
    {#if field.type === "files"}
      <label for={controlID}>{@render fieldTitle()}</label>
      <input bind:this={fileElement} class="visually-hidden" id={controlID} type="file" tabindex="-1" multiple={field.max_files === undefined || BigInt(field.max_files) > 1n} accept={field.media_types?.join(",")} disabled={busy || saving}
        onchange={(event) => { const files = Array.from(event.currentTarget.files ?? []); event.currentTarget.value = ""; void uploadFiles(files) }} />
      <div class="cluster">
        <button class="secondary small" type="button" disabled={busy || saving || onUpload === undefined} onclick={() => fileElement?.click()}>Add files</button>
        {#if saving}<span class="spinner input-action-spinner" role="status"><span class="visually-hidden">Uploading {label}</span></span>{/if}
      </div>
      {#if Array.isArray(value) && value.length > 0}
        <div class="attachment-list" aria-label={`${label} files`}>
          {#each value as file, index (file.id)}
            <span class="attachment-chip badge"><span class="attachment-chip-icon"><Paperclip size={14} strokeWidth={2} aria-hidden="true" /></span><span class="attachment-chip-label"><span>{file.name}</span><small>{file.size} bytes</small></span>
              <button class="icon inline secondary" type="button" aria-label={`Remove ${file.name}`} disabled={busy || saving} onclick={() => removeFile(index)}><X size={14} strokeWidth={2} aria-hidden="true" /></button>
            </span>
          {/each}
        </div>
      {/if}
      {#if fileError !== ""}<p class="field-help" role="alert">{fileError}</p>{/if}
    {:else if field.type === "boolean" && !field.optional}
      <label class="choice" for={controlID}>
        <input id={controlID} type="checkbox" checked={value === true} disabled={busy} onchange={(event) => void saveSet(path, event.currentTarget.checked)} />
        {@render fieldTitle()}
        {#if saving}<span class="spinner input-action-spinner" role="status"><span class="visually-hidden">Saving {label}</span></span>{/if}
      </label>
    {:else}
      <label for={controlID}>{@render fieldTitle()}</label>
      <InputControl loading={saving} loadingLabel={`Saving ${label}`} trailing={hasClearButton ? clearAction : undefined}>
        {#if field.type === "boolean"}
          <SelectControl trailing={saving}>
            <select id={controlID} disabled={busy} value={value === undefined ? "" : String(value)} onchange={(event) => {
              if (event.currentTarget.value === "") void saveRemove(path)
              else void saveSet(path, event.currentTarget.value === "true")
            }}>
              <option value="">Unanswered</option><option value="true">Yes</option><option value="false">No</option>
            </select>
          </SelectControl>
        {:else if field.type === "options"}
          <SelectControl trailing={saving}>
            <select id={controlID} disabled={busy} value={typeof value === "string" ? value : ""} onchange={(event) => {
              if (event.currentTarget.value === "") void saveRemove(path)
              else void saveSet(path, event.currentTarget.value)
            }}>
              <option value="">Select an option</option>
              {#each field.choices ?? [] as choice (choice)}<option value={choice}>{choice}</option>{/each}
            </select>
          </SelectControl>
        {:else}
          <input bind:this={inputElement} id={controlID} type="text" inputmode={field.type === "number" ? "decimal" : undefined} value={typeof value === "string" ? value : ""} disabled={busy}
            onchange={(event) => void saveSet(path, event.currentTarget.value)} />
        {/if}
      </InputControl>
    {/if}
  </div>
{/if}
