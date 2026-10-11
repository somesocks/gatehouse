import { describe, expect, it, vi } from "vitest";
import type { ApplicationRuntime } from "./runtime.svelte";
import { systemNavigation, workspaceNavigation } from "./navigation";

function createRuntime(systemAccess: "available" | "denied") {
  return {
    access: {
      state: {
        systemAccess,
        workspaces: [
          { id: "wsp_one", name: "Workspace One" },
          { id: "wsp_two", name: "Workspace Two" },
        ],
      },
    },
    auth: {
      state: {
        claims: {
          principal: { name: "User" },
        },
      },
    },
    navigate: vi.fn(),
    requireLogin: vi.fn(),
  } as unknown as ApplicationRuntime;
}

describe("sidebar navigation", () => {
  it("shows the authorized system destination in the workspace selector", () => {
    const runtime = createRuntime("available");
    const navigation = workspaceNavigation(
      runtime,
      { id: "wsp_one", name: "Workspace One" },
      "chats",
    );

    expect(navigation.sections).toHaveLength(1);
    expect(navigation.sections[0]?.items.map((item) => item.icon)).toEqual([
      "overview",
      "chats",
      "projects",
      "groups",
    ]);
    expect(navigation.workspaceSwitcher?.administration?.active).toBe(false);
    navigation.workspaceSwitcher?.administration?.onSelect();
    expect(runtime.navigate).toHaveBeenCalledWith("/app/system");
  });

  it("adds the workspace selector to system navigation", () => {
    const runtime = createRuntime("available");
    const navigation = systemNavigation(runtime, "principals");

    expect(navigation.workspaceSwitcher?.currentID).toBeNull();
    expect(navigation.workspaceSwitcher?.currentName).toBe(
      "System administration",
    );
    expect(navigation.workspaceSwitcher?.administration?.active).toBe(true);
    expect(navigation.sections[0]?.items.every((item) => item.icon)).toBe(true);
    expect(
      navigation.sections[0]?.items.some((item) => item.icon === "workspaces"),
    ).toBe(true);

    navigation.workspaceSwitcher?.onSelect("wsp/two");
    expect(runtime.navigate).toHaveBeenCalledWith("/app/wsp/wsp%2Ftwo");
  });

  it("omits the system destination without system access", () => {
    const runtime = createRuntime("denied");
    const navigation = workspaceNavigation(runtime, {
      id: "wsp_one",
      name: "Workspace One",
    });

    expect(navigation.workspaceSwitcher?.administration).toBeUndefined();
  });
});
