import { afterEach, describe, expect, it, vi } from "vitest";
import { clearAccessToken } from "./api";
import {
  createSystemWorkspace,
  createSystemWorkspacePrincipalGrant,
  fetchSystemWorkspaceAgents,
  fetchSystemWorkspacePrincipalOptions,
  fetchSystemWorkspacePrincipals,
  fetchSystemWorkspaceStorageBindings,
  fetchSystemWorkspace,
  fetchSystemWorkspaces,
  removeSystemWorkspacePrincipalGrants,
  updateSystemWorkspace,
} from "./system";

afterEach(() => {
  clearAccessToken();
  vi.unstubAllGlobals();
});

describe("system workspace API", () => {
  it("lists and creates workspaces through the system administration endpoint", async () => {
    const request = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(null, { status: 200 }),
    );
    vi.stubGlobal("fetch", request);

    await fetchSystemWorkspaces("research", "wsp_cursor");
    await createSystemWorkspace({
      alias: "research",
      name: "Research",
      enabled: true,
    });
    await fetchSystemWorkspace("wsp_detail");
    await updateSystemWorkspace("wsp_detail", {
      name: "Research Lab",
      enabled: false,
    });

    expect(request.mock.calls[0][0]).toBe(
      "/api/v1/system/workspaces?limit=50&name=research&cursor=wsp_cursor",
    );
    expect(request.mock.calls[1][0]).toBe("/api/v1/system/workspaces");
    expect(request.mock.calls[1][1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        alias: "research",
        name: "Research",
        enabled: true,
      }),
    });
    expect(request.mock.calls[2][0]).toBe(
      "/api/v1/system/workspaces/wsp_detail",
    );
    expect(request.mock.calls[3][0]).toBe(
      "/api/v1/system/workspaces/wsp_detail",
    );
    expect(request.mock.calls[3][1]).toMatchObject({
      method: "PATCH",
      body: JSON.stringify({ name: "Research Lab", enabled: false }),
    });
  });

  it("loads paginated workspace principals and manages direct grants", async () => {
    const request = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(null, { status: 200 }),
    );
    vi.stubGlobal("fetch", request);

    await fetchSystemWorkspacePrincipals("wsp_detail", "alice", "prn_cursor");
    await fetchSystemWorkspacePrincipalOptions("wsp_detail", "bob", "");
    await createSystemWorkspacePrincipalGrant(
      "wsp_detail",
      "prn_alice",
      "guest",
    );
    await removeSystemWorkspacePrincipalGrants("wsp_detail", "prn_alice");

    expect(request.mock.calls[0][0]).toBe(
      "/api/v1/system/workspaces/wsp_detail/principals?limit=50&name=alice&cursor=prn_cursor",
    );
    expect(request.mock.calls[1][0]).toBe(
      "/api/v1/system/workspaces/wsp_detail/principal-options?limit=50&name=bob",
    );
    expect(request.mock.calls[2][0]).toBe(
      "/api/v1/system/workspaces/wsp_detail/principals",
    );
    expect(request.mock.calls[2][1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({ principal: "prn_alice", role: "guest" }),
    });
    expect(request.mock.calls[3][0]).toBe(
      "/api/v1/system/workspaces/wsp_detail/principals/prn_alice",
    );
    expect(request.mock.calls[3][1]).toMatchObject({ method: "DELETE" });
  });

  it("loads only a bounded page of bindings for a selected workspace", async () => {
    const request = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(null, { status: 200 }),
    );
    vi.stubGlobal("fetch", request);

    await fetchSystemWorkspaceAgents("wsp_detail", "assistant", "wag_cursor");
    await fetchSystemWorkspaceStorageBindings(
      "wsp_detail",
      "archive",
      "stp_cursor",
    );

    expect(request.mock.calls[0][0]).toBe(
      "/api/v1/system/workspace-agents?workspace=wsp_detail&limit=50&name=assistant&cursor=wag_cursor",
    );
    expect(request.mock.calls[1][0]).toBe(
      "/api/v1/system/workspace-storage-providers?workspace=wsp_detail&limit=50&name=archive&cursor=stp_cursor",
    );
  });
});
