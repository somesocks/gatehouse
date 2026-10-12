import { describe, expect, it } from "vitest"
import { importClientModule } from "../testing/client-module"

describe("client-reactive resource lifecycle", () => {
  it("distinguishes empty success, reload, and reload failure without dropping the snapshot", async () => {
    const { createAsyncResource } = await importClientModule(
      new URL("./load-state.svelte.ts", import.meta.url),
    )
    const resource = createAsyncResource()
    resource.context("owner")
    expect(resource.lifecycle.loading).toBe(true)
    await resource.load(async () => [])
    expect(resource.lifecycle.state.hasLoaded).toBe(true)
    expect(resource.value).toEqual([])
    let reject!: (error: Error) => void
    const pending = resource.load(
      () =>
        new Promise((_, fail) => {
          reject = fail
        }),
    )
    expect(resource.lifecycle.loading).toBe(false)
    expect(resource.lifecycle.reloading).toBe(true)
    reject(new Error("Offline"))
    expect(await pending).toBe(false)
    expect(resource.lifecycle.status).toBe("ready")
    expect(resource.value).toEqual([])
    expect(resource.lifecycle.state.error).toBe("Offline")
    await resource.load(async () => ["updated"])
    expect(resource.value).toEqual(["updated"])
    expect(resource.lifecycle.state.error).toBe("")
  })
  it("rejects late success, failure and finalization after replacement or disposal", async () => {
    const { createAsyncResource } = await importClientModule(
      new URL("./load-state.svelte.ts", import.meta.url),
    )
    const resource = createAsyncResource()
    resource.context("first")
    let finish!: (value: string) => void
    const old = resource.load(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    resource.context("second")
    await resource.load(async () => "current")
    finish("stale")
    expect(await old).toBe(false)
    expect(resource.value).toBe("current")
    let reject!: (error: Error) => void
    const pending = resource.load(
      () =>
        new Promise((_, fail) => {
          reject = fail
        }),
    )
    resource.stop()
    reject(new Error("stale failure"))
    expect(await pending).toBe(false)
    expect(resource.lifecycle.state.error).toBe("")
    expect(resource.lifecycle.state.pending).toBe(false)
  })
})
