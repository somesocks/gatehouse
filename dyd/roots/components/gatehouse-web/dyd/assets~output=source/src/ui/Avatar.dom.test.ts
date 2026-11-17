import { mount, tick, unmount } from "svelte"
import { expect, it } from "vitest"
import Avatar from "./Avatar.svelte"

it("falls back to the actor icon when an avatar image fails", async () => {
  const target = document.createElement("div")
  document.body.append(target)
  const component = mount(Avatar, {
    target,
    props: { kind: "group", label: "Team", src: "/avatar.png" },
  })
  try {
    await tick()
    const image = target.querySelector("img")!
    expect(image.getAttribute("src")).toBe("/avatar.png")
    image.dispatchEvent(new Event("error"))
    await tick()
    expect(target.querySelector("img")).toBeNull()
    expect(target.querySelector(".lucide-users-round")).not.toBeNull()
    expect(
      target.querySelector('[role="img"]')?.getAttribute("aria-label"),
    ).toBe("Team")
  } finally {
    await unmount(component)
    target.remove()
  }
})
